package extension

import (
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/qimu/integrity"
)

// ContributionSource binds a value to the contribution that authored it, even
// when the application's default selection later points elsewhere.
type ContributionSource struct {
	InstallationID string `json:"installationId"`
	Release        string `json:"release"`
	Contribution   string `json:"contribution"`
}

func (s ContributionSource) Validate() error {
	if _, err := identity.ParseInstallation(s.InstallationID); err != nil {
		return err
	}
	if _, err := integrity.ParseDigest(s.Release); err != nil {
		return err
	}
	if !identity.ValidDeclarationID(s.Contribution) {
		return errors.New("source: invalid contribution")
	}
	return nil
}

// A contract reference lets archive readers interpret the captured bytes after the author is withdrawn.
type AnnotationSource struct {
	ContributionSource
	ContractReference string `json:"contractRef"`
}

func (s AnnotationSource) Validate() error {
	if s.ContractReference == "" {
		return errors.New("annotation: missing contract reference")
	}
	return s.ContributionSource.Validate()
}
