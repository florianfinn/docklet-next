// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 contract foundation; no proxy effects.

package proxy

import "time"

type CertificateEpoch struct {
	ContainerID string `json:"containerId"`
	StartedAt   string `json:"startedAt"`
}
type CertificateRoute struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	SNI        string `json:"sni"`
	EntryPoint string `json:"entryPoint"`
}
type CertificatePeer struct {
	Subject   string    `json:"subject" maxLength:"256"`
	Issuer    string    `json:"issuer" maxLength:"256"`
	SANs      []string  `json:"sans" maxItems:"32"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	SHA256    string    `json:"sha256" pattern:"^[a-f0-9]{64}$"`
}
type CertificateValidity string

const (
	CertificateValid       CertificateValidity = "valid"
	CertificateExpired     CertificateValidity = "expired"
	CertificateNotYetValid CertificateValidity = "not-yet-valid"
)

type CertificateObservation struct {
	ObservedAt    time.Time           `json:"observedAt"`
	InstanceID    string              `json:"instanceId"`
	Epoch         CertificateEpoch    `json:"epoch"`
	Route         CertificateRoute    `json:"route"`
	Peer          CertificatePeer     `json:"peer"`
	ChainTrusted  bool                `json:"chainTrusted"`
	HostnameMatch bool                `json:"hostnameMatch"`
	Validity      CertificateValidity `json:"validity" enum:"valid,expired,not-yet-valid"`
	Error         string              `json:"error,omitempty" maxLength:"256"`
}
type CertificateCheckRequest struct {
	File       string `json:"file" minLength:"1" maxLength:"1024"`
	Router     string `json:"router" minLength:"1" maxLength:"128"`
	SourceHash string `json:"sourceHash" pattern:"^[a-f0-9]{64}$"`
	Host       string `json:"host" minLength:"1" maxLength:"253"`
}
type ProbeClient string

const (
	HelperClient ProbeClient = "helper"
	WgetClient   ProbeClient = "wget"
)

type ProbeEvidence struct {
	Client  ProbeClient `json:"client" enum:"helper,wget"`
	ImageID string      `json:"imageId,omitempty"`
	Reason  Reason      `json:"reason,omitempty"`
}

type HostCertificateCheckRequest struct {
	Instance Instance `json:"instance"`
	CertificateCheckRequest
}
