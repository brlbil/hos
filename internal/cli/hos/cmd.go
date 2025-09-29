// SPDX-License-Identifier: MIT

// Package cmd provides the HOS client CLI implementation.
// It handles commands for managing pools, objects, users, and clusters
// across distributed HOS servers with configuration and authentication support.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type flags struct {
	user            string
	impersonateUser string
	cluster         string
	servers         []string
	debug           bool
	ignoreMissing   bool
}

var (
	configFile string
	userID     string
	clientConf *Config
	cmdFlags   = new(flags)
	hosClient  *client.Client
	vp         = newWiper()

	clientOptions       = []client.Options{}
	ignoredErrorOptions = []client.Options{}
)

func NewHosClientCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hos",
		Short: "Home Object Storage Client",
		Long: `Home Object Storage provides very simple object storage
command line client tool to create, read, delete, and update Pools and Objects 
and to manage users on HOS servers
`,

		SilenceErrors: true,
		SilenceUsage:  true,

		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.HasParent() {
				return nil
			}

			switch cmd.Name() {
			case "completion", "help":
				return nil
			}

			if err := loadConfig(); err != nil {
				return err
			}

			if cmdFlags.impersonateUser != "" {
				if _, ok := clientConf.GetUser("admin"); !ok {
					return errors.New(`to use -I (--impersonate) another user admin user is required
admin user does not exist in the configuration`)
				}
				cmdFlags.user = "admin"
				clientOptions = append(clientOptions, client.OnBehalf(cmdFlags.impersonateUser))
			}

			if cmdFlags.ignoreMissing {
				ignoredErrorOptions = append(clientOptions, client.IgnoreErrors(hos.ErrNotExist))
			}

			// set hos client
			var err error
			hosClient, err = newClient(cmdFlags.user, cmdFlags.cluster, cmdFlags.servers...)
			if err != nil {
				return err
			}

			// fix userID if impersonated user is set
			if cmdFlags.impersonateUser != "" {
				userID = id.Gen(cmdFlags.impersonateUser)
			}

			return nil
		},

		Run: func(cmd *cobra.Command, _ []string) {
			_ = cmd.Help()
		},
	}

	cobra.OnInitialize(func() {
		if configFile == "" {
			homeDirectory, err := homedir.Dir()
			if err != nil {
				fatal(err)
			}

			configFile = filepath.Join(homeDirectory, ".hos", "config")

			// if not exists create config dir
			if err := os.MkdirAll(filepath.Dir(configFile), 0o750); err != nil {
				fatal(err)
			}
		}

		// create the config file if not exist
		if _, err := os.Stat(configFile); err != nil {
			if err := os.WriteFile(configFile, []byte("{}"), 0o600); err != nil {
				fatal(err)
			}
		}

		vp.SetConfigFile(configFile)
		vp.SetConfigType("yaml")

		if err := vp.ReadInConfig(); err != nil {
			fatal(err)
		}
	})

	cmd.PersistentFlags().StringVarP(&configFile, "config-file", "F", "", "file to read the configuration (default is $HOME/.hos/config)")
	cmd.PersistentFlags().BoolVarP(&cmdFlags.debug, "debug", "d", false, "enable debug logging")
	cmd.PersistentFlags().StringVarP(&cmdFlags.user, "user", "u", "", "user name than will be used instead of the default configuration")
	cmd.PersistentFlags().StringVarP(&cmdFlags.impersonateUser, "impersonate", "I", "", "impersonate a user, requires admin account.")
	cmd.PersistentFlags().StringVarP(&cmdFlags.cluster, "cluster", "c", "",
		"cluster name than will be used instead of the default configuration")
	cmd.PersistentFlags().StringArrayVar(&cmdFlags.servers, "servers", []string{}, `servers to run commands against. 
servers should be member of the selected cluster or default configured cluster.`)

	cmd.MarkFlagsMutuallyExclusive("user", "impersonate")

	cmd.AddCommand(
		newInitCmd(),
		newUploadCmd(),
		newDownloadCmd(),
		newRemoveCmd(),
		newListCmd(),
		newFindCmd(),
		newStatCmd(),
		newMakePoolCmd(),
		newLinkPoolCmd(),
		newMoveCmd(),
		newCatCmd(),
		newLabelCmd(),
		newPermCmd(),
		newAttrCmd(),
		newUserCmd(),
		newKeyCmd(),
		newConfigCmd(),
		newExpCmd(),
	)

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	return cmd
}

func loadConfig() error {
	err := vp.Unmarshal(&clientConf)
	if err != nil {
		return err
	}
	if clientConf.Defaults == nil {
		clientConf.Defaults = &DefaultConfig{}
	}
	return nil
}

func newWiper() *viper.Viper {
	vp := viper.New()
	vp.SetEnvPrefix("HOS")
	vp.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	vp.AutomaticEnv()

	return vp
}

// fatal prints to stderr and exists
func fatal(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}
