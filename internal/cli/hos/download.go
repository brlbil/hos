// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/progress"
	"github.com/brlbil/hos/pkg/client"
	"github.com/spf13/cobra"
)

func newDownloadCmd() *cobra.Command {
	var (
		createPath bool
		useID      bool
		silent     bool
		labels     []string
	)

	parseDestination := func(args []string) (string, string, error) {
		destination := ""
		if len(args) == 1 {
			logDebug("func: parseDestination, getting current working dir")
			currentDir, err := os.Getwd()
			if err != nil {
				return "", "", fmt.Errorf("getting current working directory failed: %w", err)
			}
			destination = currentDir
		} else {
			destination = args[1]
		}

		fileInfo, err := os.Stat(destination)
		if err == nil {
			if fileInfo.IsDir() {
				logDebug("func: parseDestination, destPath: %s, isDir: true", destination)
				return destination, "", nil
			}
			return "", "", fmt.Errorf("destination path %s is not a directory", destination)
		}

		directory, fileName := filepath.Split(destination)
		if _, err := os.Stat(directory); err != nil {
			return "", "", fmt.Errorf("destination path %s is not a directory", directory)
		}

		logDebug("func: parseDestination, destPath: %s, dir: %s, file: %s", destination, directory, fileName)
		return directory, fileName, nil
	}

	cmd := &cobra.Command{
		Use:     "download",
		Aliases: []string{"dn", "dwn"},
		Short:   "download objects",
		Long: `download one or multiple objects to the given path
if no destination directory is given, the current directory is used

when only one object is downloaded, it can be saved with a new name
by specifying destination path as dest_dir/<new_name>

creating directory path is the default behavior
if object name has /some/path/obj, /some/path directory will be created
this behavior can be disabled by setting -C (--create-path) flag to false

Examples:
#download an object to the current directory, file will be saved as 2007/image.jpeg
hos download Images/2007/image.jpeg

#download an object to the current working directory with a different name
hos download Images/2007/image.jpeg ./img.jpg

#download objects with globbing to local Img directory, they will be saved as Img/Holidays/2001/{rest of the path}
hos dn Images/Holidays/2011/... Img

#download objects that have k=v label to local Img directory
hos dn Images Img -l k==v

#download an object by id to tmp/ folder
hos dn --id de067da0/d3bc5787 tmp/ 
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 1 {
				return fmt.Errorf("hos dn POOL/OBJECT\nexpected a pool/object as an argument, got %d args", argsLength)
			}
			if argsLength := len(args); argsLength > 2 {
				return fmt.Errorf(`hos dn POOL/OBJECT DEST_DIR
expected a pool/object and destination directory as arguments, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			poolObject := len(labels) > 0
			result, err := parseArg(userID, args[0], &argFlags{id: useID, poolObj: poolObject, labels: labels})
			if err != nil {
				return err
			}

			destinationDir, newFileName, err := parseDestination(args)
			if err != nil {
				return err
			}

			if newFileName != "" && len(result.options) > 0 {
				return fmt.Errorf("new destination file %s name cannot be used when multiple files selected, '%s...'",
					newFileName, result.objPath)
			}
			if newFileName != "" && createPath && strings.Contains(result.objPath, "/") {
				return fmt.Errorf("object name pattern %s has path {/} structure, "+
					"new destination file name %s cannot be used while --create-path option is set", result.objPath, newFileName)
			}

			var objects []hos.Object
			if len(result.options) > 0 {
				options := append(result.options, clientOptions...)
				objects, err = hosClient.ListObjects(cmd.Context(), result.poolID, options...)
				if err != nil {
					return err
				}
			} else {
				objects = []hos.Object{{ID: result.objID, PoolID: result.poolID}}
			}

			pool, err := hosClient.GetPool(cmd.Context(), result.poolID, clientOptions...)
			if err != nil {
				return err
			}

			// check for encryption
			var downloadOptions []client.Options
			if pool.Encrypted {
				encryptionKey, err := keyFromEnv(constant.EnvPassword)
				if err != nil {
					return err
				}
				if len(encryptionKey) == 0 {
					encryptionKey, err = genEncKey("", nil)
					if err != nil {
						return err
					}
				}
				downloadOptions = append(clientOptions, client.EncryptionKey(encryptionKey))
			} else {
				downloadOptions = clientOptions
			}

			for _, object := range objects {
				if err := download(cmd, &object, destinationDir, newFileName, createPath, silent, downloadOptions); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", []string{},
		"label selector to filter on, supports '==' and '!=' (e.g. -l key1==value1,key2!=value2)")
	cmd.Flags().BoolVarP(&createPath, "create-path", "C", true, "create directory path if file name has directory structure")
	cmd.Flags().BoolVar(&silent, "silent", false, "do not show progress bar for the operations")

	return cmd
}

func download(
	cmd *cobra.Command,
	object *hos.Object,
	destinationDir,
	newFileName string,
	createPath bool,
	silent bool,
	options []client.Options,
) error {
	objectContent, err := hosClient.GetContent(cmd.Context(), object.PoolID, object.ID, options...)
	if err != nil {
		return err
	}

	directory, fileName := path.Split(objectContent.Name)
	destinationFile := fileName
	if newFileName != "" {
		destinationFile = newFileName
	}

	if createPath {
		directory = filepath.Join(destinationDir, directory)
		if err := os.MkdirAll(directory, 0o770); err != nil {
			return err
		}
		destinationFile = filepath.Join(directory, destinationFile)
	} else {
		destinationFile = filepath.Join(destinationDir, destinationFile)
	}

	writeFile, err := os.Create(destinationFile)
	if err != nil {
		return err
	}
	defer writeFile.Close()

	var reader io.ReadCloser
	if !silent {
		progressBar := progress.New(objectContent.Name, objectContent.Size)
		reader = progressBar.Add(objectContent.ServerAddr(), objectContent)
		defer progressBar.Remove()
		defer progressBar.Wait()
	} else {
		reader = objectContent
	}
	defer reader.Close()

	if _, err = io.Copy(writeFile, io.LimitReader(reader, objectContent.Size)); err != nil {
		return err
	}

	return nil
}
