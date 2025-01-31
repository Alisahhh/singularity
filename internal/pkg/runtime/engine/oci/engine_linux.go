// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"github.com/sylabs/singularity/internal/pkg/runtime/engine"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config"
	ociserver "github.com/sylabs/singularity/internal/pkg/runtime/engine/oci/rpc/server"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/singularity/rpc/server"
)

// EngineOperations describes a runtime engine.
type EngineOperations struct {
	CommonConfig *config.Common `json:"-"`
	EngineConfig *EngineConfig  `json:"engineConfig"`
}

// InitConfig stores the pointer to config.Common
func (e *EngineOperations) InitConfig(cfg *config.Common) {
	e.CommonConfig = cfg
}

// Config returns a pointer to a singularity.EngineConfig literal as a
// config.EngineConfig interface. This pointer gets stored in the Engine.Common
// field.
func (e *EngineOperations) Config() config.EngineConfig {
	return e.EngineConfig
}

func init() {
	engine.RegisterOperations(
		Name,
		&EngineOperations{
			EngineConfig: &EngineConfig{},
		},
	)

	ocimethods := new(ociserver.Methods)
	ocimethods.Methods = new(server.Methods)
	engine.RegisterRPCMethods(
		Name,
		ocimethods,
	)
}
