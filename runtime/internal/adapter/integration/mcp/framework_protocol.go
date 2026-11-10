package mcp

import (
	"context"
	"errors"
	"io"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/process"
	"github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Process exports are Flame's frozen business mapping. Frame alone admits the
// neutral service contract and owns the attached process and its release proof.
type processExport struct {
	Name     string                              `json:"name"`
	Contract plugincontract.CapabilityDescriptor `json:"contract"`
}

type processConnection struct {
	source    *ProcessSource
	setup     *frame.Setup
	maximum   int
	exports   map[string]processExport
	contracts map[string]*plugincontract.Capability
	required  []pluginmcp.CapabilityExport
	session   *sdk.ClientSession
}

func (s *ProcessSource) Profile() frame.Profile { return pluginmcp.ProcessProfile }

func (s *ProcessSource) Prepare(setup *frame.Setup, metadata process.ExportMetadata, maximum int) (process.Connection, error) {
	if setup == nil || maximum <= 0 || len(metadata) != len(setup.Unit().Exports) {
		return nil, frame.ErrContractMismatch
	}
	connection := &processConnection{source: s, setup: setup, maximum: maximum,
		exports: make(map[string]processExport), contracts: make(map[string]*plugincontract.Capability)}
	physical := map[string]*plugincontract.Capability{}
	for _, export := range setup.Unit().Exports {
		encoded, found := metadata[export.Name]
		if !found {
			return nil, frame.ErrContractMismatch
		}
		var projection processExport
		if err := value.DecodeValue(encoded.Bytes(), &projection, "mcp process export"); err != nil {
			return nil, err
		}
		if !plugincontract.ValidExportName(projection.Name) {
			return nil, frame.ErrContractMismatch
		}
		contract, err := plugincontract.AdmitCapability(projection.Contract)
		if err != nil {
			return nil, err
		}
		declared, err := process.DeclaredCapability(setup, export.Contract)
		if err != nil || declared.Identity() != contract.Service().Identity() {
			return nil, errors.Join(frame.ErrContractMismatch, err)
		}
		connection.exports[export.Name], connection.contracts[export.Name] = projection, contract
		if original := physical[projection.Name]; original != nil {
			if original.Identity() != contract.Identity() {
				return nil, frame.ErrContractMismatch
			}
			continue
		}
		physical[projection.Name] = contract
		connection.required = append(connection.required, pluginmcp.CapabilityExport{Name: projection.Name, Contract: contract.Descriptor()})
	}
	return connection, nil
}

func (c *processConnection) Connect(ctx context.Context, channel io.ReadWriteCloser) ([]frame.ExportValue, error) {
	client, err := pluginmcp.NewProcessClient()
	if err != nil {
		return nil, err
	}
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: channel, Writer: channel, MaxLineLength: c.maximum}, nil)
	c.session = session
	if err != nil {
		return nil, err
	}
	if err := pluginmcp.VerifyCapabilities(ctx, session, c.required); err != nil {
		return nil, err
	}
	contributions, metadata, err := c.source.Configure(ctx, c.setup, session)
	if err != nil {
		return nil, err
	}
	if err := pluginmcp.SetUp(ctx, session, contributions, metadata); err != nil {
		return nil, err
	}
	if err := c.setup.OnActivate(func(ctx context.Context) error { return pluginmcp.Activate(ctx, session) }); err != nil {
		return nil, err
	}
	null, err := value.EncodeJSON[any](nil)
	if err != nil {
		return nil, err
	}
	values := make([]frame.ExportValue, 0, len(c.contracts))
	for _, export := range c.setup.Unit().Exports {
		binding, err := pluginmcp.NewCapabilityBinding(session, c.exports[export.Name].Name, c.contracts[export.Name])
		if err != nil {
			return nil, err
		}
		values = append(values, frame.ExportValue{Name: export.Name, Contract: export.Contract, Value: binding, Metadata: null})
	}
	return values, nil
}

func (c *processConnection) Close(context.Context) error {
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}
