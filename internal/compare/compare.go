// SPDX-License-Identifier: MIT

// Package compare provides comparison functions for HOS entities.
// It enables sorting of pools and objects by name.
package compare

import "github.com/brlbil/hos"

// Pool compares two pools by name for sorting
func Pool(a, b hos.Pool) int {
	if a.Name < b.Name {
		return -1
	}
	if a.Name > b.Name {
		return 1
	}
	return 0
}

// Object compares two objects by name for sorting
func Object(a, b hos.Object) int {
	if a.Name < b.Name {
		return -1
	}
	if a.Name > b.Name {
		return 1
	}
	return 0
}
