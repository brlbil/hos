// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/dir"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/iofactory"
	"github.com/brlbil/hos/internal/progress"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	var (
		useID       bool
		recursive   bool
		skipExist   bool
		silent      bool
		contentType string
		labels      map[string]string
	)

	cmd := &cobra.Command{
		Use:     "upload",
		Aliases: []string{"up", "upl"},
		Short:   "upload file(s) as objects to a pool",
		Long: `upload one or multiple files as objects to a pool

if -R (--recursive) flag is set, object name also includes its relative path
if --id flag is set, destination pool's ID is expected instead of its name

content-type of the objects can be overwritten by --content-type option

labels can be set on objects with -l (--label) flag

if an object exists, upload fails
if -S (--skip-exist) flag is set, existing objects are skipped

Examples:
#upload files to a pool
hos upload notes.docx *.txt Documents 

#upload files to a pool with ID
hos up notes.docx *.txt de067da0 --id

#upload files to a pool and set labels to them
hos up notes.docx *.txt Documents -l project=icarus -l color=green

#upload file tree recursively to a pool
hos up -R Notes/ Documents

#upload file tree recursively to a pool and skip existing objects
hos up -S -R Notes/ Documents
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 2 {
				return fmt.Errorf("hos up FILE... POOL\nexpected at least a file and a pool as arguments, got only %d args", argsLength)
			}
			return nil
		},

		PreRunE: func(_ *cobra.Command, _ []string) error {
			for labelKey, labelValue := range labels {
				if err := validate.Label(labelKey); err != nil {
					return err
				}
				if err := validate.LabelValue(labelValue); err != nil {
					return err
				}
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			poolArgument := args[len(args)-1]
			result, err := parseArg(userID, poolArgument, &argFlags{id: useID, pool: true})
			if err != nil {
				return err
			}

			pathInfos, err := dir.Walk(contentType, recursive, args[:(len(args)-1)]...)
			if err != nil {
				return err
			}

			getOptions := append(clientOptions, client.IgnoreErrors(hos.ErrNotExist))
			// get pool
			pool, err := hosClient.GetPool(cmd.Context(), result.poolID, getOptions...)
			if err != nil {
				return err
			}

			options := append([]client.Options{}, clientOptions...)
			if pool.Encrypted {
				// will be populated
				encryptionKey, err := keyFromEnv(constant.EnvPassword)
				if err != nil {
					return err
				}

				keys, err := hosClient.ListKeys(cmd.Context(), client.WarnErrors(hos.ErrNotExist))
				if err != nil {
					return err
				}

				// no keys, lets create one
				if len(keys) == 0 {
					// check for create key environment value
					createKeyFromEnv := strings.ToLower(os.Getenv(constant.EnvCreateKey)) == "y"

					if !createKeyFromEnv || len(encryptionKey) == 0 {
						confirmed, err := askForConfirmation("No encryption keys exist on the servers, do you want to create one now")
						if err != nil {
							return err
						}
						if !confirmed {
							return errors.New(`cannot continue without an encryption key, you need to create a key,
with 'hos key add' or by selecting 'y' for key creation step`)
						}
						_, encryptionKey, err = genEncKeyConfirm(false)
						if err != nil {
							return err
						}
					}
					if err := hosClient.CreateKey(cmd.Context(), encryptionKey); err != nil {
						return err
					}
				}
				// if there is already
				if len(keys) > 0 {
					if len(encryptionKey) > 0 {
						encryptionKey, err = genEncKey("", nil)
						if err != nil {
							return err
						}
					}

					keyID, _, _ := enc.ID(userID, encryptionKey)
					keyExists := false
					for _, key := range keys {
						if key.Signature == keyID {
							keyExists = true
							break
						}
					}
					if !keyExists {
						return fmt.Errorf("key with signature %s... is %w", keyID[:8], hos.ErrNotExist)
					}
				}
				options = append(options, client.EncryptionKey(encryptionKey))
			}

			for _, pathInfo := range pathInfos {
				objectID := id.Gen(result.poolID, pathInfo.Name)
				_, err := hosClient.GetObject(cmd.Context(), result.poolID, objectID, clientOptions...)
				if err != nil && !errors.Is(err, hos.ErrNotExist) {
					return err
				} else if err == nil {
					if skipExist {
						continue
					} else {
						return fmt.Errorf("%s is %w", pathInfo.Name, hos.ErrExist)
					}
				}

				object := hos.Object{
					PoolID:      result.poolID,
					Name:        pathInfo.Name,
					ContentType: pathInfo.ConType,
					Size:        pathInfo.Size,
				}
				if len(labels) > 0 {
					object.Labels = labels
				}

				var (
					readClosers iofactory.ReadClosers
					progressBar *progress.Progress
				)

				if !silent {
					progressBar = progress.New(object.Name, object.Size)
					readClosers = progress.ProgressReadClosers(pathInfo.ReadCloser, progressBar)
				} else {
					readClosers = pathInfo.ReadCloser
				}

				if _, err := hosClient.CreateObject(context.Background(), &object, readClosers, options...); err != nil {
					if !silent {
						progressBar.Wait()
						progressBar.Remove()
					}
					return err
				}
				if !silent {
					progressBar.Wait()
					progressBar.Remove()
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&contentType, "content-type", "", "", "set object content type instead of determining from file")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "follow directory structure recursively, set object name as relative file path")
	cmd.Flags().BoolVarP(&skipExist, "skip-exist", "S", false, "skip already existing objects")
	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool ID instead of its name")
	cmd.Flags().BoolVar(&silent, "silent", false, "do not show progress bar for the operations")
	cmd.Flags().StringToStringVarP(&labels, "label", "l", map[string]string{}, "set labels on objects")

	return cmd
}
