// Copyright (c) 2018, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// +build !linux OR !apparmor

package apparmor

import (
	"fmt"
)

// Enabled returns if apparmor is whether enabled/supported or not
func Enabled() bool {
	return false
}

// LoadProfile returns error for unsupported platform
func LoadProfile(profile string) error {
	return fmt.Errorf("apparmor is not supported by OS")
}
