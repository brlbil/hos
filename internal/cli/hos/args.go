// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
)

func logDebug(format string, values ...any) {
	if !cmdFlags.debug {
		return
	}
	log.Printf(format, values...)
}

func parsePoolArg(userID, argument string, argumentIsID bool) (string, error) {
	logDebug("func: parsePoolArg, arg: %s, id: %v", argument, argumentIsID)
	if argumentIsID {
		if err := validate.ID(argument); err != nil {
			return "", errors.New(strings.ReplaceAll(err.Error(), "id", "pool id"))
		}
		return argument, nil
	}
	if err := validate.Pool(argument); err != nil {
		return "", err
	}
	return id.Gen(userID, argument), nil
}

func recvOpts(recursive bool) []client.Options {
	if recursive {
		return []client.Options{filter.NamePrefix("")}
	}
	return []client.Options{}
}

func parseLabels(labels []string) ([]client.Options, error) {
	labelOptions := []filter.Label{}
	for _, label := range labels {
		labelFilter, err := validate.ParseLabelSelector(label)
		if err != nil {
			return []client.Options{}, err
		}
		labelOptions = append(labelOptions, labelFilter)
	}
	options := []client.Options{}
	if len(labelOptions) > 0 {
		options = append(options, filter.Labels(labelOptions))
	}
	return options, nil
}

type argRes struct {
	poolID   string
	poolName string
	objID    string
	objPath  string
	cluster  string
	dstUser  string
	options  []client.Options
}

type argFlags struct {
	labels    []string
	recursive bool
	id        bool
	pool      bool
	poolObj   bool
	copy      bool
	userAt    bool
}

func parseArg(userID, argument string, flags *argFlags) (result *argRes, err error) {
	defer func() {
		logDebug("func: parseArg, return_res: %+v", result)
	}()

	// this is special case only for copy
	hasCluster := strings.Contains(argument, ":")
	if flags.copy && (hasCluster || flags.userAt) {
		if flags.id {
			logDebug("func: parseArg, arg %s, copy: true, user_at: %v", argument, flags.userAt)
			err = fmt.Errorf("id only and cluster cannot be used at the same time")
			return
		}

		// lets cut object part if there is one
		argument, objectName, found := strings.Cut(argument, "/")
		if found {
			if validateErr := validate.Object(objectName); validateErr != nil {
				err = validateErr
				return
			}
		}

		if !hasCluster {
			user, pool, parseErr := validate.ParseUserPool(argument)
			if parseErr != nil {
				err = parseErr
				return
			}
			return &argRes{dstUser: user, poolName: pool, objPath: objectName}, nil
		}

		user, cluster, pool, parseErr := validate.ParseUserClusterPool(argument)
		if parseErr != nil {
			err = parseErr
			return
		}

		result = &argRes{dstUser: user, cluster: cluster, poolName: pool, objPath: objectName}
		return
	}

	if flags.pool {
		logDebug("func: parseArg, arg %s, pool_only: true, user_at: %v", argument, flags.userAt)
		if flags.id && flags.userAt {
			err = fmt.Errorf("id only and user@ cannot be used at the same time")
			return
		}

		poolID, parseErr := parsePoolArg(userID, argument, flags.id)
		if parseErr != nil && !flags.userAt {
			err = parseErr
			return
		}

		poolName := ""
		if flags.userAt {
			user, pool, parseErr := validate.ParseUserPool(argument)
			if parseErr != nil {
				err = parseErr
				return
			}
			userIDGenerated := id.Gen(user)
			poolID = id.Gen(userIDGenerated, pool)
			poolName = pool
		}

		options := recvOpts(flags.recursive)
		labelOptions, parseErr := parseLabels(flags.labels)
		if parseErr != nil {
			err = parseErr
			return
		}
		options = append(options, labelOptions...)

		result = &argRes{poolID: poolID, poolName: poolName, options: options}
		return
	}

	if flags.poolObj {
		logDebug("func: parseArg, arg %s, maybe_pool: true", argument)
		poolID, parseErr := parsePoolArg(userID, argument, flags.id)
		if parseErr == nil {
			result = &argRes{poolID: poolID}
			if len(flags.labels) > 0 {
				labelOptions, labelErr := parseLabels(flags.labels)
				if labelErr != nil {
					return nil, labelErr
				}
				result.options = labelOptions
				return
			}
			result.options = recvOpts(flags.recursive)
			return
		}
	}

	if flags.id {
		logDebug("func: parseArg, arg %s, object: true, id_only: true, ", argument)
		poolID, objectID, parseErr := validate.ParsePoolObjID(argument)
		if parseErr != nil {
			err = parseErr
			return
		}
		result = &argRes{poolID: poolID, objID: objectID}
		return
	}

	logDebug("func: parseArg, arg %s, object: true", argument)
	poolName, parseErr := validate.ParsePoolDot(argument)
	if parseErr == nil {
		logDebug("func: parseArg, arg %s, machPoolGlob: true", argument)
		result = &argRes{
			poolID:  id.Gen(userID, poolName),
			options: []client.Options{filter.NamePrefix("")},
		}
		labelOptions, labelErr := parseLabels(flags.labels)
		if labelErr != nil {
			err = labelErr
			return
		}
		result.options = append(result.options, labelOptions...)
		return
	}

	poolName, objectName, parseErr := validate.ParsePoolObj(argument)
	if parseErr != nil {
		err = parseErr
		return
	}

	poolID := id.Gen(userID, poolName)
	objectID := id.Gen(poolID, objectName)
	objectPath := objectName
	options := []client.Options{}
	if strings.HasSuffix(objectName, "...") {
		objectID = ""
		prefix := strings.TrimSuffix(objectName, "...")
		objectPath = prefix
		logDebug("func: parseArg, arg %s, globing: true, prefix: %s", argument, prefix)
		options = append(options, filter.NamePrefix(prefix))
		labelOptions, labelErr := parseLabels(flags.labels)
		if labelErr != nil {
			err = labelErr
			return
		}
		options = append(options, labelOptions...)
	}

	result = &argRes{poolID: poolID, objID: objectID, objPath: objectPath, options: options}
	return
}
