package httporigin

import (
	"net/url"
)

type Origin struct {
	scheme string
	host   string
}

func (o Origin) String() string { return o.scheme + "://" + o.host }

func Parse(raw string) (Origin, error) {
	endpoint, err := ParseEndpoint(raw)
	if err != nil {
		return Origin{}, err
	}
	return endpoint.Origin(), nil
}

func FromURL(u *url.URL) (Origin, error) {
	endpoint, err := endpointFromURL(u)
	if err != nil {
		return Origin{}, err
	}
	return endpoint.Origin(), nil
}

func Same(left, right string) bool {
	leftOrigin, err := Parse(left)
	if err != nil {
		return false
	}
	rightOrigin, err := Parse(right)
	return err == nil && leftOrigin == rightOrigin
}
