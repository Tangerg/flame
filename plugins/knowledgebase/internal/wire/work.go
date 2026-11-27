package wire

import "github.com/Tangerg/flame/plugins/researchartifact"

const (
	AcceptWork = "flame.knowledgebase/accept-work.v1"
	ReadWork   = "flame.knowledgebase/read-work.v1"
	ListWork   = "flame.knowledgebase/list-work.v1"
)

type ClaimRef struct {
	ID       string `json:"id" jsonschema:"minLength=1,maxLength=128"`
	Revision int64  `json:"revision" jsonschema:"minimum=1"`
}

type ArtifactRef struct {
	InstallationID string `json:"installationId"`
	ID             string `json:"id" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Revision       string `json:"revision" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Object         string `json:"object,omitempty" jsonschema:"maxLength=2048"`
}

type ResearchMethod struct {
	Name         string `json:"name" jsonschema:"minLength=1,maxLength=128"`
	Instructions string `json:"instructions" jsonschema:"minLength=1,maxLength=8192"`
}

type WorkAccept struct {
	Project          string         `json:"project"`
	ID               string         `json:"id" jsonschema:"minLength=1,maxLength=128"`
	ExpectedRevision int64          `json:"expectedRevision" jsonschema:"minimum=0"`
	Title            string         `json:"title" jsonschema:"minLength=1,maxLength=128"`
	Method           ResearchMethod `json:"method"`
	Claims           []ClaimRef     `json:"claims" jsonschema:"minItems=1,maxItems=16"`
	Artifact         ArtifactRef    `json:"artifact"`
}

type WorkRead struct {
	Project  string `json:"project"`
	ID       string `json:"id" jsonschema:"minLength=1,maxLength=128"`
	Revision int64  `json:"revision,omitempty" jsonschema:"minimum=0"`
}

type WorkReceipt struct {
	ID            string `json:"id"`
	Revision      int64  `json:"revision"`
	MethodVersion string `json:"methodVersion"`
}

type WorkSummary struct {
	WorkReceipt
	Title string `json:"title"`
}

type WorkView struct {
	WorkSummary
	State    string                     `json:"state"`
	Method   *ResearchMethod            `json:"method,omitzero"`
	Claims   []ClaimRead                `json:"claims"`
	Artifact *ArtifactRef               `json:"artifact,omitzero"`
	Evidence *researchartifact.Evidence `json:"evidence,omitzero"`
}
