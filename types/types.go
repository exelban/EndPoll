package types

import (
	"time"
)

// StatusType - host status type
// HostType - host type
type StatusType string
type HostType string

const (
	Unknown  StatusType = "unknown"
	UP       StatusType = "up"
	DEGRADED StatusType = "degraded"
	DOWN     StatusType = "down"

	HttpType  HostType = "http"
	MongoType HostType = "mongo"
	ICMPType  HostType = "icmp"
)

// Tag - color tag structure for Service
type Tag struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Status - status structure for Service
type Status struct {
	Value     StatusType `json:"value"`
	Timestamp time.Time  `json:"timestamp"`
}

// HttpResponse - response from the http dial
type HttpResponse struct {
	Timestamp time.Time     `json:"timestamp"`
	Time      time.Duration `json:"time"`
	Code      int           `json:"code,omitempty"`
	Body      string        `json:"body,omitempty"`

	OK         bool       `json:"-"`
	Bytes      []byte     `json:"-"`
	Status     bool       `json:"status,omitempty"`
	StatusType StatusType `json:"statusType,omitempty"`

	DNS           time.Duration `json:"DNS,omitempty"`
	TLSHandshake  time.Duration `json:"TLSHandshake,omitempty"`
	Connect       time.Duration `json:"connect,omitempty"`
	TTFB          time.Duration `json:"TTFB,omitempty"`
	SSLCertExpiry *time.Time `json:"SSLExpiry,omitempty"`
	SSLIssuer     string     `json:"SSLIssuer,omitempty"`
	TLSVersion    string     `json:"TLSVersion,omitempty"`

	IsAggregated bool    `json:"isAggregated"`
	Uptime       float64 `json:"uptime,omitempty"` // aggregation uptime
	Count        int     `json:"count,omitempty"`  // aggregation count
}

// Aggregation - aggregation structure for the history per day
type Aggregation struct {
	ResponseTime time.Duration `json:"responseTime"`
	Count        int           `json:"count"`
	Uptime       float64       `json:"uptime"`
	TS           time.Time     `json:"timestamp"`
}

// Incident - host incident structure
type Incident struct {
	ID       int             `json:"id"`
	Text     string          `json:"-"`
	Details  IncidentDetails `json:"details"`
	Start    string          `json:"-"`
	End      string          `json:"-"`
	Duration string          `json:"-"`
	StartTS  time.Time       `json:"startTS"`
	EndTS    *time.Time      `json:"endTS,omitempty"`
}

type IncidentDetails struct {
	StatusCode int       `json:"statusCode"`
	StatusText string    `json:"-"`
	Response   string    `json:"response,omitempty"`
	TS         time.Time `json:"ts"`
}
