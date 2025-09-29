// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos/internal/out"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/client"
	"github.com/spf13/cobra"
)

func newClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"cls"},
		Short:   "manage cluster configuration",
	}

	cmd.AddCommand(
		newAddClusterCmd(),
		newRemoveClusterCmd(),
		newListClustersCmd(),
		newSetDefaultsCmd("cluster"),
		newCordonCmd(),
		newUncordonCmd(),
	)

	return cmd
}

func addServers(cluster string, servers []string) error {
	if err := validate.Cluster(cluster); err != nil {
		return err
	}

	for _, serverAddress := range servers {
		certificate, err := client.GetCertificate(serverAddress)
		if err != nil {
			return err
		}
		serverConfig := &client.ServerConfig{Address: serverAddress, Certificate: certificate}
		if err := clientConf.AddServer(cluster, serverConfig); err != nil {
			return err
		}
	}
	return nil
}

func newAddClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "add new cluster",
		Long: `add new cluster to the configuration file
accepts server addresses as command line arguments
server CA certificates are downloaded and added to the configuration file

for an existing cluster entry, existing configuration is merged with the new one

Examples:
#add new cluster to the config
hos conf cluster add home_cluster 1.2.3.4:1981 1.2.3.5:1981

Examples:
#add new servers and merge to the same cluster config
hos conf cluster add home_cluster 1.2.3.{5,6,7}:1981
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 2 {
				return fmt.Errorf(`hos conf cluster add CLUSTER [SERVER...]
expected a cluster name and at least one server address as arguments, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			for _, address := range args[1:] {
				if err := validate.Address(address); err != nil {
					return err
				}
			}
			if err := addServers(args[0], args[1:]); err != nil {
				return err
			}

			vp.Set("clusters", clientConf.Clusters)
			return vp.WriteConfig()
		},
	}

	return cmd
}

type clusterServer struct {
	Name    string   `print:"default"`
	Default string   `print:"default"`
	Servers []string `print:"default"`
}

func newListClustersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "print cluster information",
		Long: `print cluster information from the configuration file

Examples:
#list clusters
hos config cluster ls
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 0 {
				return fmt.Errorf("hos conf cluster list\nexpected no arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, _ []string) error {
			clusters := []clusterServer{}
			for _, cluster := range clientConf.Clusters {
				clusterInfo := clusterServer{Name: cluster.Name, Servers: []string{}}
				if clientConf.Defaults.Cluster == cluster.Name {
					clusterInfo.Default = "X"
				}

				for _, server := range cluster.Servers {
					address := server.Address
					if server.Cordoned {
						address = fmt.Sprintf("**%s**", address)
					}
					clusterInfo.Servers = append(clusterInfo.Servers, address)
				}

				clusters = append(clusters, clusterInfo)
			}

			return out.Print(clusters, "default")
		},
	}

	return cmd
}

func newRemoveClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "remove a cluster",
		Long: `remove a cluster from the configuration file

Examples:
#remove a cluster from configuration
hos config cluster rm cluster1
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf(`hos conf cluster remove CLUSTER
expected a cluster name as an argument, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			clusterFound := false
			for i, cluster := range clientConf.Clusters {
				if cluster.Name == args[0] {
					clusterFound = true
					clientConf.Clusters = append(clientConf.Clusters[:i], clientConf.Clusters[i+1:]...)

					if clientConf.Defaults.Cluster == args[0] {
						clientConf.Defaults.Cluster = ""
					}
				}
			}

			if !clusterFound {
				return fmt.Errorf("cluster %s does not exist in the config", args[0])
			}

			vp.Set("clusters", clientConf.Clusters)
			vp.Set("defaults", clientConf.Defaults)
			return vp.WriteConfig()
		},
	}

	return cmd
}

func cordon(cordonEnabled bool, args []string) error {
	clusterFound, serverFound := false, false
	for i, cluster := range clientConf.Clusters {
		if cluster.Name != args[0] {
			continue
		}
		clusterFound = true
		for j, server := range cluster.Servers {
			if server.Address != args[1] {
				continue
			}
			serverFound = true
			clientConf.Clusters[i].Servers[j].Cordoned = cordonEnabled
			break
		}
	}

	if !clusterFound {
		return fmt.Errorf("cluster %s does not exist in the config", args[0])
	}

	if !serverFound {
		return fmt.Errorf("server %s does not exist in cluster %s config", args[1], args[0])
	}

	vp.Set("clusters", clientConf.Clusters)
	return vp.WriteConfig()
}

func newCordonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cordon",
		Short: "cordon a server",
		Long: `temporarily disable a server from the cluster configuration
requests will not be sent to the cordoned server

Examples:
#cordon a server from the cluster configuration
hos config cluster cordon cluster1 server1
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 2 {
				return fmt.Errorf(`hos conf cluster cordon CLUSTER SERVER
expected cluster and server names as arguments, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			return cordon(true, args)
		},
	}

	return cmd
}

func newUncordonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uncordon",
		Short: "uncordon a server",
		Long: `enable a previously cordoned server in the cluster configuration
requests will resume being sent to the server

Examples:
#uncordon a server from the cluster configuration
hos config cluster uncordon cluster server
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 2 {
				return fmt.Errorf(`hos conf cluster uncordon CLUSTER SERVER
expected cluster and server names as arguments, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(_ *cobra.Command, args []string) error {
			return cordon(false, args)
		},
	}

	return cmd
}
