package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// Peer represents the subset of a NetBird peer needed to reference it
type Peer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

// PeerResolver turns opaque peer IDs into readable Terraform references.
//
// Peers are enrolled by the NetBird client, so they are not manageable
// resources. Routes and network routers still have to point at one, and a bare
// ID says nothing about which machine it is. This emits a netbird_peer data
// source looked up by name, and hands back a reference to it.
type PeerResolver struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	idToPeer        map[string]Peer
	nameCounts      map[string]int    // peer name -> how many peers carry it
	registered      map[string]string // peer ID -> data source name
	usedNames       map[string]string // data source name -> peer ID
}

// NewPeerResolver creates a new peer resolver
func NewPeerResolver(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *PeerResolver {
	return &PeerResolver{
		service:         service,
		terraformWriter: terraformWriter,
		idToPeer:        make(map[string]Peer),
		nameCounts:      make(map[string]int),
		registered:      make(map[string]string),
		usedNames:       make(map[string]string),
	}
}

// Load fetches the peer list so IDs can be resolved to names
func (r *PeerResolver) Load() error {
	var peers []Peer
	err := r.service.Get("/api/peers", &peers)
	if err != nil {
		return fmt.Errorf("failed to fetch peers: %w", err)
	}

	for _, peer := range peers {
		r.idToPeer[peer.ID] = peer
		r.nameCounts[peer.Name]++
	}

	fmt.Printf("Loaded %d peers for reference resolution\n", len(peers))
	return nil
}

// Reference returns a Terraform reference for a peer ID, registering the
// backing data source the first time the peer is seen. Falls back to the raw
// ID when the peer is unknown, so nothing is silently dropped.
func (r *PeerResolver) Reference(peerID string) any {
	if peerID == "" {
		return nil
	}

	if name, exists := r.registered[peerID]; exists {
		return lib.RawValue(fmt.Sprintf("data.netbird_peer.%s.id", name))
	}

	peer, known := r.idToPeer[peerID]
	if !known || peer.Name == "" {
		fmt.Printf("  Warning: peer %s not found, keeping the raw ID\n", peerID)
		return peerID
	}

	// An ambiguous name would resolve to the wrong machine, which is worse than
	// an opaque ID, so keep the ID in that case.
	if r.nameCounts[peer.Name] > 1 {
		fmt.Printf("  Warning: %d peers are named %q, keeping the raw ID for %s\n", r.nameCounts[peer.Name], peer.Name, peerID)
		return peerID
	}

	name := lib.SanitizeResourceName(peer.Name)
	if name == "" {
		name = "peer_" + lib.ShortID(peerID)
	}
	// Two peers may share a name, so keep data source names unique.
	if existingID, taken := r.usedNames[name]; taken && existingID != peerID {
		name = name + "_" + lib.ShortID(peerID)
	}

	r.registered[peerID] = name
	r.usedNames[name] = peerID

	r.terraformWriter.AddDataSource("peer", name, map[string]any{
		"name": peer.Name,
	})

	return lib.RawValue(fmt.Sprintf("data.netbird_peer.%s.id", name))
}
