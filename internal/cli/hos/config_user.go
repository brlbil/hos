// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/brlbil/hos/internal/out"
	"github.com/brlbil/hos/pkg/crypto"
	"github.com/spf13/cobra"
)

func newUserConfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "manage user configuration",
	}

	cmd.AddCommand(
		newAddUserConfCmd(),
		newRemoveUserConfCmd(),
		newListUsersConfCmd(),
		newSetDefaultsCmd("user"),
	)

	return cmd
}

type userKey struct {
	Name      string `print:"default"`
	Default   string `print:"default"`
	PublicKey string `print:"default"`
}

func newAddUserConfCmd() *cobra.Command {
	var (
		privateKeyFilePath string
		overwrite          bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "add a new user",
		Long: `add a new user to the configuration file
with no flags a new private key is created for the user

private key can be provided from a file with --prv-key-from-file flag
existing user's private key can be overwritten with --overwrite-key flag 

Examples:
#add a new user in configuration and provide the private key from a file
hos config user add user1 --prv-key-from-file ./private.key

Examples:
#add a new user in configuration, create private key
hos config user add user2 
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf(`hos conf user add USER
expected a user name as an argument, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			privateKey, publicKey := "", ""
			if privateKeyFilePath != "" {
				fileData, err := os.ReadFile(privateKeyFilePath)
				if err != nil {
					return fmt.Errorf("reading file %s failed: %w", privateKeyFilePath, err)
				}
				privateKey = strings.Trim(string(fileData), " \n")
			}

			if privateKey == "" {
				publicKeyData, privateKeyData, err := crypto.GenerateKey()
				if err != nil {
					return fmt.Errorf("generating key pair failed: %w", err)
				}

				privateKey = privateKeyData.String()
				publicKey = publicKeyData.String()
			}

			if err := clientConf.AddUser(&UserConfig{Name: args[0], PrivateKey: privateKey}, overwrite); err != nil {
				return err
			}

			vp.Set("users", clientConf.Users)
			if err := vp.WriteConfig(); err != nil {
				return fmt.Errorf("writing config failed: %w", err)
			}

			if publicKey != "" {
				fmt.Println(publicKey)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&privateKeyFilePath, "prv-key-from-file", "", "private key file")
	cmd.Flags().BoolVar(&overwrite, "overwrite-key", false, "overwrite the private key of the user")

	return cmd
}

func newListUsersConfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list user information",
		Long: `list user information from the configuration file

Examples:
#list users
hos config user ls
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 0 {
				return fmt.Errorf("hos conf user list\nexpected no arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, _ []string) error {
			users := []userKey{}

			for _, user := range clientConf.Users {
				privateKey, err := crypto.ParsePrivateKey(user.PrivateKey)
				if err != nil {
					return err
				}

				publicKey, err := privateKey.PublicKey()
				if err != nil {
					return err
				}

				userKeyInfo := userKey{Name: user.Name, PublicKey: publicKey.String()}
				if clientConf.Defaults.User == user.Name {
					userKeyInfo.Default = "X"
				}

				users = append(users, userKeyInfo)
			}

			return out.Print(users, "default")
		},
	}

	return cmd
}

func newRemoveUserConfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "remove a user",
		Long: `remove a user from the configuration file

Examples:
#remove a user from configuration
hos config user rm user1
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf(`hos conf user remove USER
expected a user name as an argument, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			userFound := false
			for i, user := range clientConf.Users {
				if user.Name == args[0] {
					userFound = true
					clientConf.Users = append(clientConf.Users[:i], clientConf.Users[i+1:]...)

					if clientConf.Defaults.User == args[0] {
						clientConf.Defaults.User = ""
					}
					break
				}
			}

			if !userFound {
				return fmt.Errorf("user %s does not exist in the configuration", args[0])
			}

			vp.Set("users", clientConf.Users)
			vp.Set("defaults", clientConf.Defaults)
			return vp.WriteConfig()
		},
	}

	return cmd
}
