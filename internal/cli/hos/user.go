// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/out"
	"github.com/brlbil/hos/pkg/crypto"
	"github.com/brlbil/hos/pkg/id"
	"github.com/spf13/cobra"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "manage user accounts",
		Long: `admin account only
needs admin account to be configured
`,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			err := loadConfig()
			if err != nil {
				return err
			}
			hosClient, err = newClient("admin", cmdFlags.cluster, cmdFlags.servers...)
			if err != nil {
				return err
			}
			return nil
		},
	}

	cmd.AddCommand(
		newAddUserCmd(),
		newListUsersCmd(),
		newRemoveUserCmd(),
	)

	return cmd
}

func newAddUserCmd() *cobra.Command {
	var user *hos.User

	cmd := &cobra.Command{
		Use:   "add",
		Short: "add user to cluster",
		Long: `copy user's public key to the cluster
public key can be provided as an argument or
without an argument public key is read from configuration file

Examples:
#add user's public key
hos user add user1 VzcFQzsb6QHB0Ifqihvv46vEX+mszy3g5PNRgAuvPAU=

#add user's public key from configuration
hos user add user1
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 1 {
				return fmt.Errorf(`hos user add USER [PUB_KEY]
expected a user name as an argument, got %d args`, argsLength)
			}
			if argsLength := len(args); argsLength > 2 {
				return fmt.Errorf(`hos user add USER PUB_KEY
expected a user name and public key as arguments, got %d args`, argsLength)
			}
			return nil
		},

		PreRunE: func(cmd *cobra.Command, args []string) error {
			users, err := hosClient.ListUsers(cmd.Context())
			if err != nil {
				if errors.Is(err, hos.ErrNotInitialized) {
					return fmt.Errorf("cluster is not initialized, to initialize it run `hos init`")
				}
				return err
			}

			for _, userInfo := range users {
				if userInfo.Name == args[0] {
					user = &userInfo
					break
				}
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				publicKey crypto.PublicKey
				err       error
			)

			if len(args) == 1 {
				userConfig, ok := clientConf.GetUser(args[0])
				if !ok {
					return fmt.Errorf("user %s is not exist in configuration, to add it run `hos config user add`", args[0])
				}
				privateKey, err := crypto.ParsePrivateKey(userConfig.PrivateKey)
				if err != nil {
					return err
				}
				publicKey, _ = privateKey.PublicKey()
			} else {
				publicKey, err = crypto.ParsePublicKey(args[1])
				if err != nil {
					return err
				}
			}
			// user not exists create it
			if user == nil {
				user = &hos.User{Name: args[0], ID: id.Gen(args[0]), PublicKeys: []crypto.PublicKey{publicKey}}
				return hosClient.CreateUser(cmd.Context(), user, clientOptions...)
			}

			user.PublicKeys = []crypto.PublicKey{publicKey}
			return hosClient.EditUser(cmd.Context(), user, clientOptions...)
		},
	}

	return cmd
}

func newListUsersCmd() *cobra.Command {
	var output outType = "default"

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list users",
		Long: `list user information from the cluster

Examples:
#list users
hos user ls
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 0 {
				return fmt.Errorf("hos user list\nexpected no arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, _ []string) error {
			users, err := hosClient.ListUsers(cmd.Context())
			if err != nil {
				return err
			}

			return out.Print(users, output.String())
		},
	}

	cmd.Flags().VarP(&output, "output", "o", "output format. one of: (json, yaml, name, fields)")

	return cmd
}

func newRemoveUserCmd() *cobra.Command {
	var (
		user   *hos.User
		rmUser bool
		useID  bool
	)

	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "remove user",
		Long: `remove public key when provided as an argument
remove user when only user is given as an argument
user who has pools cannot be deleted

Examples:
#remove a user
hos user rm user1

#remove a user with ID
hos user rm --id a6b78d92

#remove user's public key
hos user rm user1 VzcFQzsb6QHB0Ifqihvv46vEX+mszy3g5PNRgAuvPAU=
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 1 {
				return fmt.Errorf(`hos user rm USER [PUB_KEY]
expected a user name as an argument, got %d args`, argsLength)
			}
			if argsLength := len(args); argsLength > 2 {
				return fmt.Errorf(`hos user rm USER PUB_KEY
expected a user name and public key as arguments, got %d args`, argsLength)
			}
			return nil
		},

		PreRunE: func(cmd *cobra.Command, args []string) error {
			pubKey := ""
			if len(args) == 2 {
				pubKey = args[1]
			}

			if pubKey == "" && args[0] == "admin" {
				return fmt.Errorf("admin user cannot be removed")
			}

			userID := args[0]
			if !useID {
				userID = id.Gen(args[0])
			}

			users, err := hosClient.ListUsers(cmd.Context())
			if err != nil {
				return err
			}

			for _, userInfo := range users {
				if userInfo.ID == userID {
					user = &userInfo
					break
				}
			}

			if user == nil {
				return fmt.Errorf("user %s %w", args[0], hos.ErrNotExist)
			}

			if pubKey == "" {
				rmUser = true
				return nil
			}

			// user admin's last remaining key cannot be deleted
			if userID == id.Admin && len(user.PublicKeys) == 1 {
				return fmt.Errorf("user admin has only one key, operation %w", hos.ErrNotAllowed)
			}

			publicKey, err := crypto.ParsePublicKey(pubKey)
			if err != nil {
				return err
			}
			keyWithPrefix := make([]byte, 33)
			keyWithPrefix[0] = '!'
			copy(keyWithPrefix[1:], publicKey)
			user.PublicKeys = []crypto.PublicKey{keyWithPrefix}

			return nil
		},

		RunE: func(cmd *cobra.Command, _ []string) error {
			if rmUser {
				return hosClient.DeleteUser(cmd.Context(), user.ID)
			}

			if user.ID == id.Admin {
				ok, err := askForConfirmation("do you want to delete admin key")
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}

			return hosClient.EditUser(cmd.Context(), user)
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use dest user's id instead of its name")

	return cmd
}
