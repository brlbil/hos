// SPDX-License-Identifier: MIT

package cmd

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/out"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
	"github.com/spf13/cobra"
)

func newKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "manage encryption keys",
		Long: `add, list and remove encryption keys
user accounts only, admin account is not allowed
`,
	}

	cmd.AddCommand(
		newAddKeyCmd(),
		newListKeysCmd(),
		newRemoveKeyCmd(),
		newBackupKeysCmd(),
		newRestoreKeyCmd(),
	)

	return cmd
}

func newAddKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "add key to cluster",
		Long: `add an encryption key to the cluster
if there are already encryption keys on the cluster,
command will ask for the current password

in case a key is missing on some of the servers or new servers are added to the cluster,
this command can add the same key to the missing servers

Examples:
#add new encryption key
$ hos key add
New Password:
Retype New Password:

#add new encryption key to the cluster with existing keys
$ hos key add
Current Password:
New Password:
Retype New Password:
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 0 {
				return fmt.Errorf("hos key add\nexpected no arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, _ []string) error {
			keys, err := hosClient.ListKeys(cmd.Context(), client.WarnErrors(hos.ErrNotExist))
			if err != nil {
				return err
			}

			var (
				currentKey []byte
				newKey     []byte
			)

			// check for create key environment value
			createKeyFromEnv := strings.ToLower(os.Getenv(constant.EnvCreateKey)) == "y"
			newKey, err = keyFromEnv(constant.EnvNewPassword)
			if err != nil {
				return err
			}

			readCurrentKey := len(keys) != 0
			if readCurrentKey {
				currentKey, err = keyFromEnv(constant.EnvPassword)
				if err != nil {
					return err
				}
			}

			if !createKeyFromEnv || len(newKey) == 0 {
				currentKey, newKey, err = genEncKeyConfirm(readCurrentKey)
				if err != nil {
					return err
				}
			}

			if readCurrentKey {
				clientOptions = append(clientOptions, client.EncryptionKey(currentKey))
			}

			return hosClient.CreateKey(cmd.Context(), newKey, clientOptions...)
		},
	}

	return cmd
}

func newListKeysCmd() *cobra.Command {
	var output outType = "default"

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "print key information",
		Long: `print key information from the cluster

Examples:
#list keys
hos key ls
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 0 {
				return fmt.Errorf("hos key list\nexpected no arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, _ []string) error {
			keys, err := hosClient.ListKeys(cmd.Context(), client.WarnErrors(hos.ErrNotExist))
			if err != nil {
				return err
			}

			return out.Print(keys, output.String())
		},
	}

	cmd.Flags().VarP(&output, "output", "o", "output format. one of: (json, yaml, name, fields)")

	return cmd
}

type keyData struct {
	UserID    string   `print:"default"`
	Signature string   `print:"default"`
	Servers   []string `print:"default"`
}

func newRestoreKeyCmd() *cobra.Command {
	var (
		filePath  string
		keyID     string
		list      bool
		serverMap map[string]string
	)

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "restore server encryption key",
		Long: `restore server encryption key from a key archive
keys are encoded with server address, in case server address has changed, address needs to be mapped to new address
key signature can be shorter, if it matches more than one, operation fails

Examples:
#list keys contained in the archive
$ hos key restore --list my_keys.tar.gz

#restore encryption key from a key archive 
$ hos key restore 036ca693f33e148626feeaa3fffe9a my_keys.tar.gz


#restore encryption key from a key archive and
#map old address 172.30.1.3:1981 to new 192.168.0.11:1981
$ hos key restore 036ca69 my_keys.tar.gz -m 192.168.0.11:1981=172.30.1.3:1981
`,

		Args: func(_ *cobra.Command, args []string) error {
			argsLength := len(args)
			if list {
				if argsLength != 1 {
					return fmt.Errorf("hos key --list ARCHIVE\nexpected one file path as the last argument, got %d args", argsLength)
				}
			} else {
				if argsLength != 2 {
					return fmt.Errorf("hos key SIGNATURE ARCHIVE\nexpected SIGNATURE and ARCHIVE as arguments, got %d args", argsLength)
				}
				keyID = args[0]
				if len(keyID) < 6 {
					return fmt.Errorf("key signature length cannot be less than 6, got %d", len(keyID))
				}
			}

			var err error
			filePath, err = filepath.Abs(args[len(args)-1])
			if err != nil {
				return fmt.Errorf("file path %s is invalid", args[0])
			}
			if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("archive file %s is not found", filePath)
			} else if err != nil {
				return err
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			keysMap, err := readKeys(file)
			if err != nil {
				return err
			}

			if list {
				keyIndexMap := map[string]int{}
				kyd := []keyData{}
				for srv, keys := range keysMap {
					for _, key := range keys {
						if index, ok := keyIndexMap[key.ID]; ok {
							kyd[index].Servers = append(kyd[index].Servers, srv)
						} else {
							kd := keyData{
								UserID:    key.UserID,
								Signature: key.ID,
								Servers:   []string{srv},
							}
							keyIndexMap[key.ID] = len(kyd)
							kyd = append(kyd, kd)
						}
					}
				}

				return out.Print(kyd, "default")
			}

			userID := id.Gen(hosClient.User())

			serverKeyMap := map[string]enc.Key{}
			for srv, keys := range keysMap {
				index := slices.IndexFunc(keys, func(key enc.Key) bool {
					return strings.HasPrefix(key.ID, keyID)
				})
				if index < 0 {
					continue
				}
				if keys[index].UserID != userID {
					return fmt.Errorf("archive user id %s is not equal to current user id %s",
						keys[index].UserID, userID)
				}
				serverKeyMap[srv] = keys[index]
			}

			if len(serverKeyMap) == 0 {
				return fmt.Errorf("key %s is not found in archive", keyID)
			}

			for oldAddress, newAddrs := range serverMap {
				if val, ok := serverKeyMap[oldAddress]; ok {
					serverKeyMap[newAddrs] = val
					delete(serverKeyMap, oldAddress)
				}
			}

			return hosClient.RestoreKey(cmd.Context(), serverKeyMap)
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "list keys contained in the archive")
	cmd.Flags().StringToStringVarP(&serverMap, "map", "m", map[string]string{}, "map old sever address to new address")

	return cmd
}

func readKeys(file io.Reader) (map[string][]enc.Key, error) {
	gr, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	ret := map[string][]enc.Key{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		dir, sig := path.Split(header.Name)
		uid, server := path.Split(strings.TrimSuffix(dir, "/"))
		uid = strings.TrimSuffix(uid, "/")

		key := enc.Key{
			UserID:    uid,
			ID:        sig,
			Data:      data,
			CreatedAt: header.ModTime,
		}
		if keys, ok := ret[server]; !ok {
			ret[server] = []enc.Key{key}
		} else {
			ret[server] = append(keys, key)
		}
	}

	return ret, nil
}

func newBackupKeysCmd() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:     "backup",
		Aliases: []string{"bk"},
		Short:   "backup server encryption keys",
		Long: `backup server encryption keys to a tar gzip archive

Examples:
#backup encryption keys to archive file 
$ hos key backup my_keys.tar.gz
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf("hos key FILE\nexpected one file path as the last argument, got %d args", argsLength)
			}

			var err error
			filePath, err = filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("file path %s is invalid", args[0])
			}
			if _, err := os.Stat(filePath); err == nil {
				return fmt.Errorf("archive file %s already exists", filePath)
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			keyData, err := hosClient.GetServerKeys(cmd.Context())
			if err != nil {
				return err
			}

			userID := id.Gen(hosClient.User())

			file, err := os.Create(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			zw := gzip.NewWriter(file)
			defer zw.Close()

			tw := tar.NewWriter(zw)
			defer tw.Close()

			for server, keys := range keyData {
				for _, key := range keys {
					hd := &tar.Header{
						Name:    path.Join(userID, server, key.ID),
						Mode:    0o600,
						ModTime: key.CreatedAt,
						Size:    int64(len(key.Data)),
						Format:  tar.FormatPAX,
					}
					if err := tw.WriteHeader(hd); err != nil {
						return fmt.Errorf("fail to write header: %w", err)
					}
					if _, err := tw.Write(key.Data); err != nil {
						return fmt.Errorf("fail to write data: %w", err)
					}
				}
			}

			return nil
		},
	}

	return cmd
}

func newRemoveKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "remove encryption key",
		Long: `remove an encryption key by its signature
the last encryption key cannot be deleted

Examples:
#remove an encryption key
hos key rm 036ca693f33e148626feeaa3fffe9a3a183077e29f5ad5a9e417b6e3d1a6ba06

#remove an encryption key with prefix of a key
hos key rm 036ca6
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf(`hos key rm SIGNATURE
expected a key signature as an argument, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			sig := strings.TrimSpace(args[0])
			// verify signature length
			if l := len(sig); l < 6 {
				return fmt.Errorf("expected minimum signature length 6, got %d", l)
			}
			return hosClient.DeleteKey(cmd.Context(), sig, clientOptions...)
		},
	}

	return cmd
}
