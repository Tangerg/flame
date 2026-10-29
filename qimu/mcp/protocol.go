// Package-level wire projections are generated from compiled protocol owners.
package mcp

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
)

//go:generate go run ./cmd/protocolgen --out ../node/src/protocol.generated.json
type ProtocolConstants struct {
	Protocol              string `json:"protocol"`
	InvokeMethod          string `json:"invokeMethod"`
	DescribeMethod        string `json:"describeMethod"`
	ValidateMethod        string `json:"validateMethod"`
	SetupMethod           string `json:"setupMethod"`
	ActivateMethod        string `json:"activateMethod"`
	CallbackMetadataKey   string `json:"callbackMetadataKey"`
	StreamMetadataKey     string `json:"streamMetadataKey"`
	CallHeader            string `json:"CallHeader"`
	InvocationHeader      string `json:"InvocationHeader"`
	InvocationMetadataKey string `json:"InvocationMetadataKey"`
	ReceiptMethod         string `json:"ReceiptMethod"`
	MaxCommandIDBytes     int    `json:"maxCommandIDBytes"`
}

func ProtocolJSON() ([]byte, error) {
	constants := ProtocolConstants{
		Protocol:              portable.Protocol,
		InvokeMethod:          InvokeMethod,
		DescribeMethod:        DescribeMethod,
		ValidateMethod:        ValidateMethod,
		SetupMethod:           SetupMethod,
		ActivateMethod:        ActivateMethod,
		CallbackMetadataKey:   CallbackMetadataKey,
		StreamMetadataKey:     StreamMetadataKey,
		CallHeader:            CallHeader,
		InvocationHeader:      InvocationHeader,
		InvocationMetadataKey: InvocationMetadataKey,
		ReceiptMethod:         ReceiptMethod,
		MaxCommandIDBytes:     MaxCommandIDBytes,
	}
	receipt, err := ReceiptQuery()
	if err != nil {
		return nil, err
	}
	receiptJSON, err := jsonvalue.Encode(receipt.Descriptor())
	if err != nil {
		return nil, err
	}
	delivery, err := StreamDeliveryContract()
	if err != nil {
		return nil, err
	}
	deliveryJSON, err := jsonvalue.Encode(delivery)
	if err != nil {
		return nil, err
	}
	type inputVector struct {
		Source    string `json:"source"`
		Canonical string `json:"canonical"`
		Digest    string `json:"digest"`
	}
	var vectors []inputVector
	for _, source := range []string{`{"z":1.0,"a":9007199254740993}`, `{"a":1e+20,"n":-0,"s":"\u0061"}`, `{"\ud83d\ude00":1,"\uffff":2,"a":[false,null]}`} {
		input, err := jsonvalue.Parse([]byte(source))
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, inputVector{Source: source, Canonical: input.String(), Digest: input.Digest()})
	}
	astral, bmp := receipt.Descriptor(), receipt.Descriptor()
	astral.Name, bmp.Name = "\U0001f600", "\uffff"
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "\ufeffcontract", Composition: portable.Collection, Scope: "installation"}, astral, bmp)
	if err != nil {
		return nil, err
	}
	contractJSON, err := jsonvalue.Encode(contract.Descriptor())
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(struct {
		Constants          ProtocolConstants `json:"constants"`
		Limits             portable.Limits   `json:"limits"`
		Receipt            string            `json:"receipt"`
		ReceiptIdentity    string            `json:"receiptIdentity"`
		StreamDelivery     string            `json:"streamDelivery"`
		Inputs             []inputVector     `json:"inputs"`
		Capability         string            `json:"capability"`
		CapabilityIdentity string            `json:"capabilityIdentity"`
	}{constants, portable.ProfileLimits(), receiptJSON.String(), receipt.contract.Identity().Digest(), deliveryJSON.String(), vectors, contractJSON.String(), contract.Identity().Digest()}, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
