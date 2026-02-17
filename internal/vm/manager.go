//go:build libvirt

// Package vm provides virtual machine management for Unraid systems via libvirt.
//
// LibvirtVMManager is the production implementation that connects to a running
// libvirt daemon via its Unix domain socket.  It satisfies the VMManager
// interface defined in types.go.
//
// Build with -tags libvirt to include the real implementation:
//
//	go build -tags libvirt ./...
//
// The go-libvirt dependency must be present in go.mod:
//
//	go get github.com/digitalocean/go-libvirt
package vm

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"
)

// ----------------------------------------------------------------------------
// Internal XML structs for parsing domain XML
// ----------------------------------------------------------------------------

// domainXML is used to unmarshal the subset of a libvirt domain XML document
// that we need for populating VMDetail.
type domainXML struct {
	XMLName xml.Name   `xml:"domain"`
	Name    string     `xml:"name"`
	UUID    string     `xml:"uuid"`
	Memory  domainMem  `xml:"memory"`
	VCPUs   int        `xml:"vcpu"`
	Devices domainDevs `xml:"devices"`
}

type domainMem struct {
	Unit  string `xml:"unit,attr"`
	Value uint64 `xml:",chardata"`
}

type domainDevs struct {
	Disks      []domainDisk `xml:"disk"`
	Interfaces []domainNIC  `xml:"interface"`
}

type domainDisk struct {
	Type   string        `xml:"type,attr"`   // "file", "block", etc.
	Device string        `xml:"device,attr"` // "disk", "cdrom", etc.
	Source domainDiskSrc `xml:"source"`
	Target domainDiskTgt `xml:"target"`
}

type domainDiskSrc struct {
	File string `xml:"file,attr"`
	Dev  string `xml:"dev,attr"`
}

type domainDiskTgt struct {
	Dev string `xml:"dev,attr"`
}

type domainNIC struct {
	Type   string         `xml:"type,attr"`
	MAC    domainNICMAC   `xml:"mac"`
	Source domainNICSrc   `xml:"source"`
	Model  domainNICModel `xml:"model"`
}

type domainNICMAC struct {
	Address string `xml:"address,attr"`
}

type domainNICSrc struct {
	Network string `xml:"network,attr"`
	Bridge  string `xml:"bridge,attr"`
}

type domainNICModel struct {
	Type string `xml:"type,attr"`
}

// ----------------------------------------------------------------------------
// LibvirtVMManager
// ----------------------------------------------------------------------------

// LibvirtVMManager implements VMManager using the go-libvirt pure-Go client.
type LibvirtVMManager struct {
	l          *libvirt.Libvirt
	socketPath string
}

// NewLibvirtVMManager dials the libvirt Unix socket at socketPath, performs
// the libvirt connect handshake, and returns a ready-to-use LibvirtVMManager.
func NewLibvirtVMManager(socketPath string) (*LibvirtVMManager, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("libvirt socket path must not be empty")
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("dial libvirt socket %q: %w", socketPath, err)
	}

	l := libvirt.New(conn)
	if err := l.Connect(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("libvirt connect: %w", err)
	}

	return &LibvirtVMManager{
		l:          l,
		socketPath: socketPath,
	}, nil
}

// Close disconnects from the libvirt daemon and releases the underlying
// network connection.
func (m *LibvirtVMManager) Close() error {
	if err := m.l.Disconnect(); err != nil {
		return fmt.Errorf("libvirt disconnect: %w", err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Interface implementation
// ----------------------------------------------------------------------------

// ListVMs returns a summary for every domain known to libvirt (active and
// inactive).
func (m *LibvirtVMManager) ListVMs(ctx context.Context) ([]VM, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}

	// Request all domains (active + inactive).
	domains, _, err := m.l.ConnectListAllDomains(1, libvirt.ConnectListDomainsActive|libvirt.ConnectListDomainsInactive)
	if err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}

	out := make([]VM, 0, len(domains))
	for _, d := range domains {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("list vms: %w", ctx.Err())
		}
		v, err := m.domainToVM(d)
		if err != nil {
			// Skip domains we cannot inspect rather than aborting the whole list.
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// InspectVM returns the full details for the named VM.  It returns an error
// containing "not found" if the VM does not exist.
func (m *LibvirtVMManager) InspectVM(ctx context.Context, name string) (*VMDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("inspect vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return nil, fmt.Errorf("vm %q not found: %w", name, err)
	}

	detail, err := m.domainToVMDetail(dom)
	if err != nil {
		return nil, fmt.Errorf("inspect vm %q: %w", name, err)
	}
	return detail, nil
}

// StartVM starts a shutoff (or paused) domain.  It returns an error containing
// "already running" if the domain is already in the running state.
func (m *LibvirtVMManager) StartVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	state, err := m.domainState(dom)
	if err != nil {
		return fmt.Errorf("start vm %q: get state: %w", name, err)
	}
	if state == VMStateRunning {
		return fmt.Errorf("vm %q already running", name)
	}

	if err := m.l.DomainCreate(dom); err != nil {
		return fmt.Errorf("start vm %q: %w", name, err)
	}
	return nil
}

// StopVM gracefully shuts down a running domain via ACPI.
func (m *LibvirtVMManager) StopVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("stop vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	if err := m.l.DomainShutdown(dom); err != nil {
		return fmt.Errorf("stop vm %q: %w", name, err)
	}
	return nil
}

// ForceStopVM destroys a domain immediately, equivalent to pulling the power
// cord.
func (m *LibvirtVMManager) ForceStopVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("force stop vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	if err := m.l.DomainDestroy(dom); err != nil {
		return fmt.Errorf("force stop vm %q: %w", name, err)
	}
	return nil
}

// PauseVM suspends a running domain.  It returns an error containing
// "not running" if the domain is not currently in the running state.
func (m *LibvirtVMManager) PauseVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("pause vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	state, err := m.domainState(dom)
	if err != nil {
		return fmt.Errorf("pause vm %q: get state: %w", name, err)
	}
	if state != VMStateRunning {
		return fmt.Errorf("vm %q not running", name)
	}

	if err := m.l.DomainSuspend(dom); err != nil {
		return fmt.Errorf("pause vm %q: %w", name, err)
	}
	return nil
}

// ResumeVM resumes a paused domain.  It returns an error containing
// "not paused" if the domain is not currently in the paused state.
func (m *LibvirtVMManager) ResumeVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("resume vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	state, err := m.domainState(dom)
	if err != nil {
		return fmt.Errorf("resume vm %q: get state: %w", name, err)
	}
	if state != VMStatePaused {
		return fmt.Errorf("vm %q not paused", name)
	}

	if err := m.l.DomainResume(dom); err != nil {
		return fmt.Errorf("resume vm %q: %w", name, err)
	}
	return nil
}

// RestartVM sends a reboot signal to the domain.
func (m *LibvirtVMManager) RestartVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("restart vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	if err := m.l.DomainReboot(dom, 0); err != nil {
		return fmt.Errorf("restart vm %q: %w", name, err)
	}
	return nil
}

// CreateVM defines a new domain from the supplied XML configuration.  The XML
// must not be empty or consist solely of whitespace.
func (m *LibvirtVMManager) CreateVM(ctx context.Context, xmlConfig string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create vm: %w", err)
	}
	if strings.TrimSpace(xmlConfig) == "" {
		return fmt.Errorf("create vm: xml config is empty")
	}

	if _, err := m.l.DomainDefineXML(xmlConfig); err != nil {
		return fmt.Errorf("create vm: %w", err)
	}
	return nil
}

// DeleteVM undefines (removes the persistent definition of) the named domain.
// It returns an error containing "not found" if the domain does not exist.
func (m *LibvirtVMManager) DeleteVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete vm: %w", err)
	}

	dom, err := m.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", name, err)
	}

	if err := m.l.DomainUndefine(dom); err != nil {
		return fmt.Errorf("delete vm %q: %w", name, err)
	}
	return nil
}

// ListSnapshots returns the metadata for every snapshot associated with
// the named VM.
func (m *LibvirtVMManager) ListSnapshots(ctx context.Context, vmName string) ([]Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	dom, err := m.l.DomainLookupByName(vmName)
	if err != nil {
		return nil, fmt.Errorf("vm %q not found: %w", vmName, err)
	}

	names, err := m.l.DomainSnapshotListNames(dom, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list snapshots for %q: %w", vmName, err)
	}

	out := make([]Snapshot, 0, len(names))
	for _, n := range names {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("list snapshots: %w", ctx.Err())
		}
		out = append(out, Snapshot{
			Name:      n,
			CreatedAt: time.Now(),
		})
	}
	return out, nil
}

// CreateSnapshot creates a new snapshot of the named VM with the given snapshot
// name.
func (m *LibvirtVMManager) CreateSnapshot(ctx context.Context, vmName, snapName string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	dom, err := m.l.DomainLookupByName(vmName)
	if err != nil {
		return fmt.Errorf("vm %q not found: %w", vmName, err)
	}

	snapXML := fmt.Sprintf("<domainsnapshot><name>%s</name></domainsnapshot>", snapName)
	if _, err := m.l.DomainSnapshotCreateXML(dom, snapXML, 0); err != nil {
		return fmt.Errorf("create snapshot %q for vm %q: %w", snapName, vmName, err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

// domainState retrieves the current VMState for a domain.
func (m *LibvirtVMManager) domainState(dom libvirt.Domain) (VMState, error) {
	state, _, err := m.l.DomainGetState(dom, 0)
	if err != nil {
		return "", fmt.Errorf("get domain state: %w", err)
	}
	return libvirtStateToVMState(libvirt.DomainState(state)), nil
}

// domainToVM builds a VM summary from a libvirt Domain.
func (m *LibvirtVMManager) domainToVM(dom libvirt.Domain) (VM, error) {
	state, err := m.domainState(dom)
	if err != nil {
		return VM{}, err
	}

	xmlDesc, err := m.l.DomainGetXMLDesc(dom, 0)
	if err != nil {
		return VM{}, fmt.Errorf("get xml desc: %w", err)
	}

	var d domainXML
	if err := xml.Unmarshal([]byte(xmlDesc), &d); err != nil {
		return VM{}, fmt.Errorf("parse domain xml: %w", err)
	}

	memory := normalizeMemoryKB(d.Memory)

	return VM{
		Name:   dom.Name,
		UUID:   formatUUID(dom.UUID),
		State:  state,
		Memory: memory,
		VCPUs:  d.VCPUs,
	}, nil
}

// domainToVMDetail builds a full VMDetail from a libvirt Domain.
func (m *LibvirtVMManager) domainToVMDetail(dom libvirt.Domain) (*VMDetail, error) {
	state, err := m.domainState(dom)
	if err != nil {
		return nil, err
	}

	xmlDesc, err := m.l.DomainGetXMLDesc(dom, 0)
	if err != nil {
		return nil, fmt.Errorf("get xml desc: %w", err)
	}

	var d domainXML
	if err := xml.Unmarshal([]byte(xmlDesc), &d); err != nil {
		return nil, fmt.Errorf("parse domain xml: %w", err)
	}

	memory := normalizeMemoryKB(d.Memory)

	detail := &VMDetail{
		VM: VM{
			Name:   dom.Name,
			UUID:   formatUUID(dom.UUID),
			State:  state,
			Memory: memory,
			VCPUs:  d.VCPUs,
		},
		XMLConfig: xmlDesc,
	}

	// Populate disks (only "disk" devices, not cdroms).
	for _, disk := range d.Devices.Disks {
		if disk.Device != "disk" && disk.Device != "" {
			continue
		}
		src := disk.Source.File
		if src == "" {
			src = disk.Source.Dev
		}
		detail.Disks = append(detail.Disks, VMDisk{
			Source: src,
			Target: disk.Target.Dev,
			Type:   disk.Type,
		})
	}

	// Populate NICs.
	for _, iface := range d.Devices.Interfaces {
		network := iface.Source.Network
		if network == "" {
			network = iface.Source.Bridge
		}
		detail.NICs = append(detail.NICs, VMNIC{
			MAC:     iface.MAC.Address,
			Network: network,
			Model:   iface.Model.Type,
		})
	}

	return detail, nil
}

// libvirtStateToVMState maps a libvirt DomainState integer to our VMState type.
func libvirtStateToVMState(s libvirt.DomainState) VMState {
	switch s {
	case libvirt.DomainRunning:
		return VMStateRunning
	case libvirt.DomainShutoff:
		return VMStateShutoff
	case libvirt.DomainPaused:
		return VMStatePaused
	case libvirt.DomainCrashed:
		return VMStateCrashed
	case libvirt.DomainPmsuspended:
		return VMStateSuspended
	default:
		return VMStateShutoff
	}
}

// normalizeMemoryKB converts a domain memory value to kilobytes.
// Libvirt XML defaults to KiB; this handles "b", "mb", and "gb" as well.
func normalizeMemoryKB(mem domainMem) uint64 {
	switch strings.ToLower(mem.Unit) {
	case "b", "bytes":
		return mem.Value / 1024
	case "mb", "mib", "m":
		return mem.Value * 1024
	case "gb", "gib", "g":
		return mem.Value * 1024 * 1024
	default:
		// Default unit in libvirt XML is KiB.
		return mem.Value
	}
}

// formatUUID converts the 16-byte UUID array from go-libvirt into the
// standard hyphenated 8-4-4-4-12 hex string representation.
func formatUUID(uuid [16]byte) string {
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16],
	)
}
