// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
)

func newClient(user, cluster string, servers ...string) (*client.Client, error) {
	if user == "" {
		user = clientConf.Defaults.User
	}
	if cluster == "" {
		cluster = clientConf.Defaults.Cluster
	}

	var serverConfigs []client.ServerConfig
	for _, clusterConfig := range clientConf.Clusters {
		if clusterConfig.Name != cluster {
			continue
		}

		if len(servers) == 0 {
			for _, server := range clusterConfig.Servers {
				if server.Cordoned {
					continue
				}
				serverConfigs = append(serverConfigs, server)
			}

			if len(serverConfigs) == 0 {
				return nil, fmt.Errorf("all servers of cluster %s are cordoned", cluster)
			}
			break
		}

		for _, serverAddress := range servers {
			serverAdded := false
			for _, server := range clusterConfig.Servers {
				if server.Address == serverAddress && !server.Cordoned {
					serverConfigs = append(serverConfigs, server)
					serverAdded = true
					break
				}
			}
			if !serverAdded {
				return nil, fmt.Errorf("server %s is not defined in configuration of cluster %s", serverAddress, clusterConfig.Name)
			}
		}
	}

	if len(serverConfigs) == 0 {
		return nil, fmt.Errorf("cluster %s is not found in the configuration", cluster)
	}

	var clientOptions []client.ConfigFunc
	userConfig, userExists := clientConf.GetUser(user)
	if !userExists {
		return nil, fmt.Errorf("user %s is not defined in the configuration", user)
	}

	clientOptions = append(clientOptions, client.SetUserKey(user, userConfig.PrivateKey))
	userID = id.Gen(user)

	if cmdFlags.debug {
		clientOptions = append(clientOptions, client.DebugLogging)
	}

	return client.New(serverConfigs, clientOptions...)
}
