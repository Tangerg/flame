package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/qimu"
	"github.com/Tangerg/qimu/portable"
	"github.com/Tangerg/qimu/process"
)

const ProcessProfile qimu.Profile = "mcp-process"

type ProcessOptions struct {
	MaxFrameBytes int
	Configure     func(context.Context, *qimu.Setup, *sdk.ClientSession) ([]SetupContribution, sdk.ParamsBase, error)
}

type processProfile struct{ options ProcessOptions }

func NewProcessDriver(cfg process.Config, options ProcessOptions) (*process.Driver[*sdk.ClientSession], error) {
	if options.MaxFrameBytes <= 0 {
		return nil, fmt.Errorf("%w: MCP maximum frame size is required", qimu.ErrInvalid)
	}
	return process.New(cfg, processProfile{options})
}

func (processProfile) Name() qimu.Profile { return ProcessProfile }

func (p processProfile) Bind(setup *qimu.Setup, program process.Program) (process.Connect[*sdk.ClientSession], error) {
	contracts, required, err := requiredProcessExports(setup, program)
	if err != nil {
		return nil, err
	}
	unit := setup.Unit()
	return func(ownedCtx context.Context, channel io.ReadWriteCloser) (*sdk.ClientSession, []qimu.ExportValue, error) {
		client, err := NewProcessClient()
		if err != nil {
			return nil, nil, err
		}
		session, err := client.Connect(ownedCtx, &sdk.IOTransport{Reader: channel, Writer: channel, MaxLineLength: p.options.MaxFrameBytes}, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := VerifyCapabilities(ownedCtx, session, required); err != nil {
			return session, nil, err
		}
		var metadata sdk.ParamsBase
		var contributions []SetupContribution
		if p.options.Configure != nil {
			contributions, metadata, err = p.options.Configure(ownedCtx, setup, session)
			if err != nil {
				return session, nil, err
			}
		} else {
			if len(unit.Imports) != 0 {
				return session, nil, errors.New("process: declared imports require an approved callback configuration")
			}
			for _, export := range unit.Exports {
				contributions = append(contributions, SetupContribution{ID: export.Name, Export: physicalExport(program, export.Name), Requires: []SetupDependency{}})
			}
		}
		if err := SetUp(ownedCtx, session, contributions, metadata); err != nil {
			return session, nil, err
		}
		if err := setup.OnActivate(func(activateCtx context.Context) error { return Activate(activateCtx, session) }); err != nil {
			return session, nil, err
		}
		values := make([]qimu.ExportValue, 0, len(unit.Exports))
		for _, export := range unit.Exports {
			binding, err := NewCapabilityBinding(session, physicalExport(program, export.Name), contracts[export.Name])
			if err != nil {
				return session, nil, err
			}
			var published any
			if value, present := program.Metadata[export.Name]; present {
				published = value
			}
			values = append(values, qimu.ExportValue{Name: export.Name, Contract: export.Contract, Metadata: published, Value: binding, Durable: true})
		}
		return session, values, nil
	}, nil
}

func requiredProcessExports(setup *qimu.Setup, program process.Program) (map[string]*portable.Capability, []CapabilityExport, error) {
	unit := setup.Unit()
	contracts := make(map[string]*portable.Capability, len(unit.Exports))
	required := make([]CapabilityExport, 0, len(unit.Exports))
	physicalContracts := make(map[string]*portable.Capability)
	for _, export := range unit.Exports {
		valueContract, err := setup.ValueContract(export.Contract)
		if err != nil {
			return nil, nil, err
		}
		descriptor, err := setup.ContractDescriptor(valueContract)
		if err != nil {
			return nil, nil, err
		}
		contract, err := portable.ParseCapability(descriptor)
		if err != nil {
			return nil, nil, err
		}
		if contract == nil || valueContract.Mode != qimu.PortableMode || valueContract.Kind != qimu.ServiceKind || contract.Descriptor().Reference != valueContract.Name || contract.Identity().Digest() != valueContract.Digest {
			return nil, nil, qimu.ErrContractMismatch
		}
		contracts[export.Name] = contract
		physical := physicalExport(program, export.Name)
		if original := physicalContracts[physical]; original != nil {
			if original.Identity() != contract.Identity() {
				return nil, nil, qimu.ErrContractMismatch
			}
		} else {
			physicalContracts[physical] = contract
			required = append(required, CapabilityExport{Name: physical, Contract: contract.Descriptor()})
		}
	}
	return contracts, required, nil
}

func physicalExport(program process.Program, logical string) string {
	if physical, found := program.Exports[logical]; found {
		return physical
	}
	return logical
}
