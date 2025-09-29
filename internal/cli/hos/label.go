// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
	"github.com/spf13/cobra"
)

type replacer struct {
	labels map[string]string
}

func (*replacer) Option() {}

func (r *replacer) Filter(a any) any {
	objects, ok := a.([]hos.Object)
	if !ok {
		return a
	}
	for i := range objects {
		objects[i].Labels = r.labels
	}
	return objects
}

func newLabelCmd() *cobra.Command {
	var (
		useID    bool
		labelSel []string
		rm       bool
	)

	cmd := &cobra.Command{
		Use:   "label",
		Short: "label pool or object(s)",
		Long: `add labels to a pool, an object, or multiple objects
key value arguments expected as key=value pairs

remove labels with --rm flag and label key

Examples:
#add labels to pool by id
hos label --id f1658489 key1=val1 key2=val2 

#add labels to object by id
hos label --id f1658489/bf82de32 key1=val1 key2=val2

#add labels to multiple objects
hos label Docs/Jan/hr... key1=val1 key2=val2

#remove labels from pool
hos label Docs --rm key1 key2

#remove labels from multiple objects
hos label Docs/Jan/hr... --rm key1 key2
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 2 {
				return fmt.Errorf(`hos label [POOL, POOL/OBJECT, POOL/OBJECT...] [KEY, KEY=VAL...]
expected a pool or pool/object and one or more keys or key=val pairs as arguments, got only %d args`, argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			labels := map[string]string{}
			if rm {
				for _, label := range args[1:] {
					if err := validate.Label(label); err != nil {
						return err
					}
					labels["!"+label] = ""
				}
			} else {
				for _, label := range args[1:] {
					key, val, err := validate.ParseLabel(label)
					if err != nil {
						return err
					}
					labels[key] = val
				}
			}

			result, err := parseArg(userID, args[0], &argFlags{id: useID, poolObj: true, labels: labelSel})
			if err != nil {
				return err
			}

			// it is only a pool
			if result.objPath == "" && len(labelSel) == 0 {
				pool := &hos.Pool{ID: result.poolID, Labels: labels}
				_, err = hosClient.EditPool(cmd.Context(), pool, clientOptions...)
				return err
			}

			var objects []hos.Object
			if len(result.options) > 0 {
				options := append(result.options, &replacer{labels: labels})
				options = append(options, clientOptions...)
				objects, err = hosClient.ListObjects(cmd.Context(), result.poolID, options...)
				if err != nil {
					return err
				}
			} else {
				objects = []hos.Object{{ID: result.objID, PoolID: result.poolID, Labels: labels}}
			}

			for _, object := range objects {
				_, err := hosClient.EditObject(cmd.Context(), &object, clientOptions...)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")
	cmd.Flags().StringArrayVarP(&labelSel, "label-selector", "l", []string{},
		"label selector to filter on, supports '==' and '!=' (e.g. -l key1==value1,key2!=value2)")
	cmd.Flags().BoolVar(&rm, "rm", false, "remove labels")

	return cmd
}
