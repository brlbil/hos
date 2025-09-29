// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/crypto"
	"github.com/brlbil/hos/pkg/id"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var (
		cluster          string
		servers          []string
		adminExist       bool
		defaultUser      string
		defaultUserExist bool
	)

	cmd := &cobra.Command{
		Use:     "init",
		Aliases: []string{"initialize"},
		Short:   "initialize cluster",
		Long: `initialize a cluster

- add admin user to configuration file
- copy admin user to uninitialized cluster
- add cluster to configuration file
- add a default user to configuration file, if selected
- copy default user to cluster, if selected

cluster name that exists in configuration cannot be used
cluster must be uninitialized, no admin account key copied before

Examples:
#initialize a cluster
hos init 10.10.10.1:1981 10.10.10.2.1981
`,

		Args: cobra.MinimumNArgs(1),

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig()
		},

		PreRunE: func(_ *cobra.Command, args []string) error {
			for _, addr := range args {
				if err := validate.Address(addr); err != nil {
					return err
				}
			}

			var err error
			// read the cluster name if it is not provided from command line
			if len(cluster) == 0 {
				cluster, err = readLine("Cluster Name")
				if err != nil {
					return err
				}
			}

			// check cluster name
			if err := validate.Cluster(cluster); err != nil {
				return err
			}

			var clusterConfig *ClusterConfig
			for _, cls := range clientConf.Clusters {
				if cls.Name == cluster {
					clusterConfig = &cls
					break
				}
			}

			if clusterConfig != nil {
				for _, address := range args {
					if i := slices.IndexFunc(clusterConfig.Servers, func(srv client.ServerConfig) bool {
						return srv.Address == address
					}); i == -1 {
						servers = append(servers, address)
					} else {
						return fmt.Errorf(" ↪️ server '%s' already exists in the configuration 🔸", cluster)
					}
				}
			}
			_, adminExist = clientConf.GetUser("admin")

			if len(defaultUser) == 0 {
				ok, err := askForConfirmation("Do you want to create a default user")
				if err != nil {
					return err
				}

				if ok {
					defUser, err := readLine("User Name")
					if err != nil {
						return err
					}
					defaultUser = defUser
				}
			}

			_, defaultUserExist = clientConf.GetUser(defaultUser)

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			if adminExist {
				fmt.Println("Add user admin ✅\n ↪️ already exists 🔸")
			} else {
				fmt.Print("Add user admin")
				_, adminPrivateKey, err := crypto.GenerateKey()
				if err != nil {
					return fmt.Errorf("\n ↪️ generating key pair failed: %w ❌", err)
				}
				if err := clientConf.AddUser(&UserConfig{Name: "admin", PrivateKey: adminPrivateKey.String()}, false); err != nil {
					return fmt.Errorf("\n ↪️ adding user admin failed: %w ❌", err)
				}
				fmt.Println(" ✅")
			}

			if defaultUser != "" {
				if defaultUserExist {
					fmt.Printf("Add user %s ✅\n ↪️ already exists 🔸\n", defaultUser)
				} else {
					fmt.Printf("Add user %s", defaultUser)
					_, userPrivateKey, err := crypto.GenerateKey()
					if err != nil {
						return fmt.Errorf("\n ↪️ generating key pair failed: %w ❌", err)
					}
					if err := clientConf.AddUser(&UserConfig{Name: defaultUser, PrivateKey: userPrivateKey.String()}, false); err != nil {
						return fmt.Errorf("\n ↪️ adding user %s failed: %w ❌", defaultUser, err)
					}
					fmt.Println(" ✅")
				}
			}

			fmt.Printf("Add servers to cluster %s", cluster)
			if err := addServers(cluster, args); err != nil {
				return fmt.Errorf("\n ↪️ adding cluster %s failed: %w ❌", cluster, err)
			}
			fmt.Println(" ✅")
			// if default cluster is not set, set this cluster as default
			if defCluster, _ := clientConf.GetDefaults(); defCluster == "" {
				_ = clientConf.SetDefaults(cluster, "") // just added cluster should not fail
				fmt.Printf("Set cluster %s as the default cluster ✅\n", cluster)
			}

			fmt.Printf("Copy admin user's key to cluster %s", cluster)
			print := true
			// construct a client
			client, err := newClient("admin", cluster, servers...)
			if err != nil {
				return fmt.Errorf("\n ↪️ initializing client failed: %w ❌", err)
			}

			users, err := client.ListUsers(cmd.Context())
			if err != nil && !errors.Is(err, hos.ErrNotInitialized) {
				if errors.Is(err, hos.ErrNotAuthorized) {
					return fmt.Errorf("\n ↪️ cluster is already initialized with another admin key ❌")
				}
				return fmt.Errorf("\n ↪️ copying user admin key failed: %w ❌", err)
			}

			// cluster is not initialized
			if errors.Is(err, hos.ErrNotInitialized) {
				adminUser, _ := clientConf.GetUser("admin")
				adminPrivateKeyObj, err := crypto.ParsePrivateKey(adminUser.PrivateKey)
				if err != nil {
					return fmt.Errorf("\n ↪️ parsing private key failed: %w ❌", err)
				}
				// can ignore error here, private key is just parsed
				adminPublicKey, _ := adminPrivateKeyObj.PublicKey()
				if err := client.EditUser(cmd.Context(),
					&hos.User{ID: id.Admin, Name: "admin", PublicKeys: []crypto.PublicKey{adminPublicKey}}); err != nil {
					return fmt.Errorf("\n ↪️ copying user admin key failed: %w ❌", err)
				}
			} else {
				fmt.Println(" ✅\n ↪️ user admin already exists and has the same public key 🔸")
				print = false
			}
			if print {
				fmt.Println(" ✅")
			}

			if defaultUser != "" {
				// is user already exists on the server
				var remoteDefaultUser *hos.User
				for _, user := range users {
					if user.Name == defaultUser {
						remoteDefaultUser = &user
					}
				}

				fmt.Printf("Copy user %s key to cluster %s", defaultUser, cluster)
				print = true
				defaultUserConfig, _ := clientConf.GetUser(defaultUser)
				defaultPrivateKeyObj, err := crypto.ParsePrivateKey(defaultUserConfig.PrivateKey)
				if err != nil {
					return fmt.Errorf("\n ↪️ parsing private key failed: %w ❌", err)
				}
				// can ignore error here, private key is just parsed
				defaultPublicKey, _ := defaultPrivateKeyObj.PublicKey()

				// user exists on the server
				if remoteDefaultUser != nil {
					addKey := true
					for _, existingPublicKey := range remoteDefaultUser.PublicKeys {
						if bytes.Equal(defaultPublicKey, existingPublicKey) {
							fmt.Printf(" ✅\n ↪️ user %s already exists and has the same public key 🔸\n", defaultUser)
							print = false
							addKey = false
						}
					}

					if addKey {
						if err := client.EditUser(cmd.Context(),
							&hos.User{ID: id.Gen(defaultUser), PublicKeys: []crypto.PublicKey{defaultPublicKey}}); err != nil {
							return fmt.Errorf("\n ↪️ copying user %s key failed: %w ❌", defaultUser, err)
						}
					}
				} else { // user not exists
					if err := client.CreateUser(cmd.Context(),
						&hos.User{ID: id.Gen(defaultUser), Name: defaultUser, PublicKeys: []crypto.PublicKey{defaultPublicKey}}); err != nil {
						return fmt.Errorf("\n ↪️ copying user %s key failed: %w ❌", defaultUser, err)
					}
				}
				if print {
					fmt.Println(" ✅")
				}
				// if default user is not set, set this user as default
				if _, confDefUser := clientConf.GetDefaults(); confDefUser == "" {
					_ = clientConf.SetDefaults("", defaultUser) // just added user should not fail
					fmt.Printf("Set user %s ad the default user ✅\n", defaultUser)
				}
			}

			// save the configuration file
			vp.Set("clusters", clientConf.Clusters)
			vp.Set("users", clientConf.Users)
			if clientConf.Defaults != nil {
				vp.Set("defaults", clientConf.Defaults)
			}
			if err := vp.WriteConfig(); err != nil {
				return fmt.Errorf("writing config failed: %w ❌", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&cluster, "cluster-name", "", "the name of the cluster")
	cmd.Flags().StringVar(&defaultUser, "default-user", "", "the name of the default user, if the user does not exist it is created")

	return cmd
}
