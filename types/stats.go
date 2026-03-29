package types

import "time"

// Point is a struct that contains the timestamp, status, and tooltip of a host. Used for the chart.
type Point struct {
	Status    StatusType
	Tooltip   *string
	Timestamp string
	TS        time.Time
}

// Chart is a struct that contains the points and intervals of a host.
type Chart struct {
	Points    []*Point
	Intervals []string
}

// SSLDetails is a struct that contains the expiration date of the SSL certificate.
// LastOutageDetails is a struct that contains the duration of the last outage.
type SSLDetails struct {
	ExpireInDays int
	ExpireTS     string
	Issuer       string
	TLSVersion   string
}
type LastOutageDetails struct {
	Duration string
	Since    string
	TS       string
}

// Details is a struct that contains the uptime and response time of a host.
type Details struct {
	Uptime       string
	ResponseTime string
	SSL          *SSLDetails
	LastOutage   *LastOutageDetails
}

// Stat is a struct that contains the stats of a host.
type Stat struct {
	ID           string
	Name         *string
	Description  *string
	Host         string
	Status       StatusType
	Uptime       int
	ResponseTime string
	Chart        Chart
	Hosts        []Stat
	Details      *Details

	Index int
}

// Stats is a struct that contains the stats of all hosts.
type Stats struct {
	IsHost    bool
	Status    StatusType
	Hosts     []Stat
	Incidents []*Incident
}
