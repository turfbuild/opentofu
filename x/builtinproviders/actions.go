// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package builtinproviders

import (
	"context"
	"fmt"
	"sort"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plugin"
	"github.com/opentofu/opentofu/internal/plugin/convert"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

// Action is one action type an action-only built-in provider serves: the
// configuration block a Terraform 1.14 `action` of that type takes, and the
// host's judgement of a configuration that decoded against it.
type Action struct {
	// Schema is the action's `config` block (x/configs.Block to a caller).
	Schema *configschema.Block
	// Validate judges a configuration of Schema's implied type. It runs at
	// validation and again at plan, so the value may still hold unknowns the
	// first time and a check that needs a known value passes those by. A
	// cty.PathError names the attribute at fault. Nil accepts everything the
	// schema does.
	Validate func(config cty.Value) (warnings []string, err error)
}

// ServeActions runs a built-in provider whose whole surface is the given
// action types — no resources, no data sources, no configuration block — as
// a go-plugin provider server over this process's stdio, the way Serve runs
// one of OpenTofu's own. It blocks until the client disconnects and must be
// the last thing the process does. The only way it returns is a definition
// it cannot serve, reported before the handshake begins.
//
// Why this is not a providers.Interface behind Serve: OpenTofu's provider
// interface, and the grpcwrap server Serve wraps it in, predate actions and
// have no method for them, while the wire protocol this module generates
// does. So the server here is written against the protocol directly, and it
// answers exactly what a plugin client asks of an action: the schema, the
// two validations, and the plan.
//
// It does not invoke. An action served this way is one the host answers
// itself — it asked for the schema and the validation, and it holds the
// invocation — so InvokeAction completes with an error saying so rather
// than doing something on the host's behalf.
func ServeActions(actions map[string]Action) error {
	server, err := actionServerFor(actions)
	if err != nil {
		return err
	}
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return server
		},
	})
	return nil
}

// actionServerFor builds the protocol-5 server for a set of definitions.
// Unexported for the reason serverFor is: tfplugin5 is an internal type.
func actionServerFor(actions map[string]Action) (tfplugin5.ProviderServer, error) {
	if len(actions) == 0 {
		return nil, fmt.Errorf("an action-only built-in provider needs at least one action type")
	}
	for name, a := range actions {
		if a.Schema == nil {
			return nil, fmt.Errorf("action type %q has no schema", name)
		}
	}
	return &actionServer{actions: actions}, nil
}

// actionServer answers what a plugin client asks of a provider that has
// only actions. Everything else is the embedded Unimplemented server's, and
// a client has no reason to ask it: the schema declares no resource, data
// source, ephemeral resource or function.
type actionServer struct {
	actions map[string]Action

	tfplugin5.UnimplementedProviderServer
}

func (s *actionServer) GetSchema(context.Context, *tfplugin5.GetProviderSchema_Request) (*tfplugin5.GetProviderSchema_Response, error) {
	resp := &tfplugin5.GetProviderSchema_Response{
		// Present and empty, as grpcwrap answers for a provider with no
		// configuration: a client reads the block without a nil check.
		Provider:                 &tfplugin5.Schema{Block: &tfplugin5.Schema_Block{}},
		ProviderMeta:             &tfplugin5.Schema{Block: &tfplugin5.Schema_Block{}},
		ResourceSchemas:          map[string]*tfplugin5.Schema{},
		DataSourceSchemas:        map[string]*tfplugin5.Schema{},
		EphemeralResourceSchemas: map[string]*tfplugin5.Schema{},
		ActionSchemas:            make(map[string]*tfplugin5.ActionSchema, len(s.actions)),
		ServerCapabilities:       &tfplugin5.ServerCapabilities{},
	}
	for name, a := range s.actions {
		resp.ActionSchemas[name] = &tfplugin5.ActionSchema{
			Schema: &tfplugin5.Schema{Block: convert.ConfigSchemaToProto(a.Schema)},
		}
	}
	return resp, nil
}

func (s *actionServer) GetResourceIdentitySchemas(context.Context, *tfplugin5.GetResourceIdentitySchemas_Request) (*tfplugin5.GetResourceIdentitySchemas_Response, error) {
	return &tfplugin5.GetResourceIdentitySchemas_Response{
		IdentitySchemas: map[string]*tfplugin5.ResourceIdentitySchema{},
	}, nil
}

// PrepareProviderConfig and Configure accept whatever arrives: there is no
// configuration block, so a client sends the empty value or nothing.
func (s *actionServer) PrepareProviderConfig(context.Context, *tfplugin5.PrepareProviderConfig_Request) (*tfplugin5.PrepareProviderConfig_Response, error) {
	return &tfplugin5.PrepareProviderConfig_Response{}, nil
}

func (s *actionServer) Configure(context.Context, *tfplugin5.Configure_Request) (*tfplugin5.Configure_Response, error) {
	return &tfplugin5.Configure_Response{}, nil
}

func (s *actionServer) ValidateActionConfig(_ context.Context, req *tfplugin5.ValidateActionConfig_Request) (*tfplugin5.ValidateActionConfig_Response, error) {
	return &tfplugin5.ValidateActionConfig_Response{Diagnostics: s.judge(req.TypeName, req.Config)}, nil
}

// PlanAction judges the configuration again — the plan's value is the more
// known of the two — and never defers: there is nothing behind this provider
// that could be unready.
func (s *actionServer) PlanAction(_ context.Context, req *tfplugin5.PlanAction_Request) (*tfplugin5.PlanAction_Response, error) {
	return &tfplugin5.PlanAction_Response{Diagnostics: s.judge(req.ActionType, req.Config)}, nil
}

func (s *actionServer) InvokeAction(req *tfplugin5.InvokeAction_Request, stream tfplugin5.Provider_InvokeActionServer) error {
	return stream.Send(&tfplugin5.InvokeAction_Event{
		Type: &tfplugin5.InvokeAction_Event_Completed_{
			Completed: &tfplugin5.InvokeAction_Event_Completed{
				Diagnostics: []*tfplugin5.Diagnostic{{
					Severity: tfplugin5.Diagnostic_ERROR,
					Summary:  "Action is not invoked by its provider",
					Detail: fmt.Sprintf("%s is an action its host answers itself; this provider serves its schema "+
						"and validates its configuration, and has nothing to invoke.", req.ActionType),
				}},
			},
		},
	})
}

func (s *actionServer) Stop(context.Context, *tfplugin5.Stop_Request) (*tfplugin5.Stop_Response, error) {
	return &tfplugin5.Stop_Response{}, nil
}

// judge decodes a configuration against its action's schema and runs the
// definition's Validate over it.
func (s *actionServer) judge(typeName string, config *tfplugin5.DynamicValue) []*tfplugin5.Diagnostic {
	a, ok := s.actions[typeName]
	if !ok {
		return []*tfplugin5.Diagnostic{{
			Severity: tfplugin5.Diagnostic_ERROR,
			Summary:  "Unsupported action type",
			Detail:   fmt.Sprintf("This provider serves no action type %q (it serves %v).", typeName, s.names()),
		}}
	}
	ty := a.Schema.ImpliedType()
	val := cty.NullVal(ty)
	var err error
	switch {
	case config == nil:
	case len(config.Msgpack) > 0:
		val, err = msgpack.Unmarshal(config.Msgpack, ty)
	case len(config.Json) > 0:
		val, err = ctyjson.Unmarshal(config.Json, ty)
	}
	if err != nil {
		return convert.AppendProtoDiag(nil, err)
	}
	if a.Validate == nil {
		return nil
	}
	warnings, err := a.Validate(val)
	var errs []error
	if err != nil {
		errs = []error{err}
	}
	return convert.WarnsAndErrsToProto(warnings, errs)
}

func (s *actionServer) names() []string {
	names := make([]string, 0, len(s.actions))
	for name := range s.actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
