package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// ReferenceResolver turns the opaque IDs that policies and reverse proxy
// targets carry into readable Terraform references.
//
// Those IDs point at either a peer or a network resource, told apart by the
// accompanying type field ("peer" versus "host"/"subnet"/"domain").
type ReferenceResolver struct {
	peers    *PeerResolver
	networks *NetworksHandler
}

// NewReferenceResolver creates a resolver over the given sources
func NewReferenceResolver(peers *PeerResolver, networks *NetworksHandler) *ReferenceResolver {
	return &ReferenceResolver{peers: peers, networks: networks}
}

// Reference resolves an ID of the given kind. It falls back to the raw ID when
// the target cannot be resolved, so a reference is never silently dropped.
func (r *ReferenceResolver) Reference(id, kind string) any {
	if id == "" {
		return nil
	}

	if kind == "peer" {
		if r.peers == nil {
			return id
		}
		return r.peers.Reference(id)
	}

	if r.networks != nil {
		if name, ok := r.networks.NetworkResourceName(id); ok {
			return lib.RawValue(fmt.Sprintf("netbird_network_resource.%s.id", name))
		}
	}

	fmt.Printf("  Warning: %s resource %s not found, keeping the raw ID\n", kind, id)
	return id
}
