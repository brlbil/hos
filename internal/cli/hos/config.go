// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/client"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"conf"},
		Short:   "manage configuration file",

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig()
		},
	}

	cmd.AddCommand(
		newClusterCmd(),
		newUserConfCmd(),
		newViewCmd(),
	)

	return cmd
}

func newSetDefaultsCmd(parentName string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-as-def",
		Aliases: []string{"def", "set-as-default"},
		Short:   fmt.Sprintf("set %s as default", parentName),
		Long: fmt.Sprintf(`set %s as default
%s must be in the configuration

Examples:
#set %s1 as the default
hos config %s def %s1
`, parentName, parentName, parentName, parentName, parentName),

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf(`hos conf %s set-as-def %s
expected a %s name, got %d args`, parentName, strings.ToUpper(parentName), parentName, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			var (
				cluster = args[0]
				user    string
			)

			if parentName == "user" {
				cluster = ""
				user = args[0]
			}

			if err := clientConf.SetDefaults(cluster, user); err != nil {
				return fmt.Errorf("setting defaults failed: %w", err)
			}

			vp.Set("defaults", clientConf.Defaults)
			if err := vp.WriteConfig(); err != nil {
				return fmt.Errorf("writing config failed: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "print configuration file content",

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 0 {
				return fmt.Errorf("hos config view\nexpected no arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, _ []string) error {
			configData, err := os.ReadFile(configFile)
			if err != nil {
				return err
			}

			fmt.Println(string(configData))

			return nil
		},
	}

	return cmd
}

// Config represents client configuration
type Config struct {
	Clusters []ClusterConfig `json:"clusters,omitempty"`
	Defaults *DefaultConfig  `json:"defaults,omitempty"`
	Users    []UserConfig    `json:"users,omitempty"`
}

type DefaultConfig struct {
	User    string `json:"user,omitempty"`
	Cluster string `json:"cluster,omitempty"`
}

type ClusterConfig struct {
	Name    string                `json:"name,omitempty"`
	Servers []client.ServerConfig `json:"servers,omitempty"`
}

// UserConfig represents server configuration
type UserConfig struct {
	Name       string `json:"name,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
}

func (c *Config) AddServer(cluster string, serverConfig *client.ServerConfig) error {
	// check the IP address is a proper one
	if _, err := netip.ParseAddrPort(serverConfig.Address); err != nil {
		return err
	}

	clusterIndex := len(c.Clusters)
	for i, clusterConfig := range c.Clusters {
		if clusterConfig.Name == cluster {
			clusterIndex = i
			break
		}
	}

	if clusterIndex == len(c.Clusters) {
		c.Clusters = append(c.Clusters,
			ClusterConfig{Name: cluster, Servers: []client.ServerConfig{}})
	}

	for _, server := range c.Clusters[clusterIndex].Servers {
		if server.Address == serverConfig.Address {
			return nil
		}
	}

	c.Clusters[clusterIndex].Servers = append(c.Clusters[clusterIndex].Servers, *serverConfig)

	return nil
}

func (c *Config) AddUser(userConfig *UserConfig, overwrite bool) error {
	if err := validate.User(userConfig.Name); err != nil {
		return err
	}

	for i, user := range c.Users {
		if user.Name != userConfig.Name {
			continue
		}

		if overwrite {
			c.Users[i].PrivateKey = userConfig.PrivateKey
			return nil
		}
		return fmt.Errorf("user %s already exists", userConfig.Name)
	}

	c.Users = append(c.Users, *userConfig)
	return nil
}

func (c *Config) GetUser(name string) (*UserConfig, bool) {
	for _, user := range c.Users {
		if user.Name == name {
			return &user, true
		}
	}
	return nil, false
}

func (c *Config) SetDefaults(cluster, user string) error {
	if c.Defaults == nil {
		c.Defaults = &DefaultConfig{}
	}

	if cluster != "" {
		clusterFound := false
		for _, clusterConfig := range c.Clusters {
			clusterFound = clusterConfig.Name == cluster
		}

		if !clusterFound {
			return fmt.Errorf("cluster %s is not in config", cluster)
		}

		c.Defaults.Cluster = cluster
	}

	if user == "" {
		return nil
	}

	if user == "admin" {
		return errors.New("admin user cannot be set as default user")
	}

	for _, userConfig := range c.Users {
		if userConfig.Name == user {
			c.Defaults.User = user
			return nil
		}
	}

	return fmt.Errorf("user %s is not found in config", user)
}

func (c *Config) GetDefaults() (cluster, user string) {
	if c.Defaults == nil {
		return
	}
	cluster = c.Defaults.Cluster
	user = c.Defaults.User
	return
}
