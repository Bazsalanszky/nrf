// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

/*
 * NRF Configuration Factory
 */

package factory

import (
	"cmp"
	"fmt"
	"os"

	"github.com/omec-project/nrf/logger"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/omec-project/util/sbiaddr"
)

const (
	NRF_EXPECTED_CONFIG_VERSION = "1.0.0"
	NRF_DEFAULT_IPV4            = "127.0.0.1"
	NRF_DEFAULT_PORT            = 29510
	NRF_DEFAULT_SCHEME          = "https"
)

type Config struct {
	Info          *Info              `yaml:"info"`
	Configuration *Configuration     `yaml:"configuration"`
	Logger        *utilLogger.Logger `yaml:"logger"`
	CfgLocation   string
}

type Info struct {
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Configuration struct {
	Sbi                   *Sbi     `yaml:"sbi,omitempty"`
	MongoDBName           string   `yaml:"MongoDBName"`
	MongoDBUrl            string   `yaml:"MongoDBUrl"`
	WebuiUri              string   `yaml:"webuiUri"`
	ServiceNameList       []string `yaml:"serviceNameList,omitempty"`
	NfKeepAliveTime       int32    `yaml:"nfKeepAliveTime,omitempty"`
	MongoDBStreamEnable   bool     `yaml:"mongoDBStreamEnable"`
	NfProfileExpiryEnable bool     `yaml:"nfProfileExpiryEnable"`
}

type Sbi struct {
	Scheme          string `yaml:"scheme"`
	TLS             *TLS   `yaml:"tls"`
	RegisterIPv4    string `yaml:"registerIPv4,omitempty"`    // IPv4 address, host name, or env var name (POD_IP, POD_IPS) used in the NRF's own URIs.
	RegisterIPv6    string `yaml:"registerIPv6,omitempty"`    // IPv6 address, host name, or env var name used in the NRF's own URIs.
	BindingAddr     string `yaml:"bindingAddr,omitempty"`     // Listen address of any family, or env var name; wins over bindingIPv4.
	BindingIPv4     string `yaml:"bindingIPv4,omitempty"`     // Legacy alias of bindingAddr.
	PreferredFamily string `yaml:"preferredFamily,omitempty"` // IPv4 (default) or IPv6: the family of the NRF's own URIs when it has both.
	Port            int    `yaml:"port,omitempty"`
}

type TLS struct {
	PEM string `yaml:"pem,omitempty"`
	Key string `yaml:"key,omitempty"`
}

func (c *Config) GetVersion() string {
	if c.Info != nil && c.Info.Version != "" {
		return c.Info.Version
	}
	return ""
}

func (c *Config) GetSbiScheme() string {
	if c.Configuration != nil && c.Configuration.Sbi != nil && c.Configuration.Sbi.Scheme != "" {
		return c.Configuration.Sbi.Scheme
	}
	return NRF_DEFAULT_SCHEME
}

func (c *Config) GetSbiPort() int {
	if c.Configuration != nil && c.Configuration.Sbi != nil && c.Configuration.Sbi.Port != 0 {
		return c.Configuration.Sbi.Port
	}
	return NRF_DEFAULT_PORT
}

// GetSbiBindingAddr returns the host:port the SBI server listens on.
// bindingAddr wins over its legacy alias bindingIPv4, either may name an
// environment variable (e.g. POD_IP), and the default host is 0.0.0.0, which
// Go opens as a dual-stack socket. An IPv6 host is bracketed.
func (c *Config) GetSbiBindingAddr() string {
	if c.Configuration == nil || c.Configuration.Sbi == nil {
		return sbiHostPort(sbiaddr.DefaultBindHost, NRF_DEFAULT_PORT)
	}
	sbi := c.Configuration.Sbi
	if name := cmp.Or(sbi.BindingAddr, sbi.BindingIPv4); name != "" {
		if value := os.Getenv(name); value != "" {
			logger.CfgLog.Infof("parsing ServerIPv4 [%s] from ENV Variable", value)
		}
	}
	bindHost, err := sbiaddr.BindHost(sbi.BindingAddr, sbi.BindingIPv4)
	if err != nil {
		logger.CfgLog.Warnf("SBI binding address: %v", err)
	}
	return sbiHostPort(bindHost, c.GetSbiPort())
}

// sbiHostPort joins the SBI binding host and port. net.JoinHostPort brackets an
// IPv6 literal ("[::]:29510"); the previous string concatenation did not, so an
// IPv6 bindingIPv4 failed with "too many colons in address". IPv4 addresses,
// hostnames and an empty host give the same string as before. A host that is
// already bracketed is unwrapped first so it is not bracketed twice.
func sbiHostPort(host string, port int) string {
	return sbiaddr.HostPort(host, port)
}

// GetSbiRegisterAddrs returns the addresses the NRF uses in its own URIs
// (ApiPrefix, Location headers) and NF services, resolved from registerIPv4
// and registerIPv6 (see sbiaddr.ResolveRegister: each setting may name an
// environment variable such as POD_IP or POD_IPS, and every value goes to
// the slot of its own family). Unusable values are dropped. When nothing is
// left, the result is NRF_DEFAULT_IPV4. As in the other NFs, a host name
// suppresses the IPv4 address. Problems are reported by LogSbiAddressWarnings.
func (c *Config) GetSbiRegisterAddrs() sbiaddr.RegisterAddrs {
	addrs, _ := c.resolveSbiRegisterAddrs()
	return addrs
}

func (c *Config) resolveSbiRegisterAddrs() (sbiaddr.RegisterAddrs, []error) {
	var registerIPv4, registerIPv6 string
	if c.Configuration != nil && c.Configuration.Sbi != nil {
		registerIPv4, registerIPv6 = c.Configuration.Sbi.RegisterIPv4, c.Configuration.Sbi.RegisterIPv6
	}
	var warnings []error
	addrs, err := sbiaddr.ResolveRegister(registerIPv4, registerIPv6)
	if err != nil {
		warnings = append(warnings, fmt.Errorf("SBI register address: %w", err))
	}
	if addrs.Empty() {
		addrs.IPv4 = NRF_DEFAULT_IPV4
	}
	if addrs.FQDN != "" && addrs.IPv4 != "" {
		warnings = append(warnings, fmt.Errorf("SBI register address: using FQDN %s, not IPv4 %s", addrs.FQDN, addrs.IPv4))
		addrs.IPv4 = ""
	}
	return addrs, warnings
}

// GetSbiPreferredFamily returns the configured preferredFamily. It is IPv4
// when the key is unset or invalid.
func (c *Config) GetSbiPreferredFamily() sbiaddr.Preference {
	pref, err := c.parseSbiPreferredFamily()
	if err != nil {
		return sbiaddr.PreferIPv4
	}
	return pref
}

func (c *Config) parseSbiPreferredFamily() (sbiaddr.Preference, error) {
	if c.Configuration == nil || c.Configuration.Sbi == nil {
		return sbiaddr.PreferIPv4, nil
	}
	return sbiaddr.ParsePreference(c.Configuration.Sbi.PreferredFamily)
}

// LogSbiAddressWarnings logs, once at startup, every problem found in the SBI
// register and family settings. The getters above ignore them silently, as
// they are called for every registration.
func (c *Config) LogSbiAddressWarnings() {
	_, warnings := c.resolveSbiRegisterAddrs()
	for _, w := range warnings {
		logger.CfgLog.Warnln(w)
	}
	if _, err := c.parseSbiPreferredFamily(); err != nil {
		logger.CfgLog.Warnf("SBI preferredFamily: %v; using IPv4", err)
	}
}

// GetSbiRegisterIP returns the host of the NRF's own URIs, unbracketed: the
// host name if one is configured, else the address of the preferred family.
// Despite its name it may be an IPv6 address.
func (c *Config) GetSbiRegisterIP() string {
	return c.GetSbiRegisterAddrs().Host(c.GetSbiPreferredFamily())
}

// GetSbiRegisterAddr returns GetSbiRegisterIP():port, with an IPv6 host in
// brackets.
func (c *Config) GetSbiRegisterAddr() string {
	return sbiaddr.HostPort(c.GetSbiRegisterIP(), c.GetSbiPort())
}

// GetSbiUri returns the NRF's own base URI, scheme://host:port, with an IPv6
// host in brackets. It is the ApiPrefix of the NRF's services and the base of
// the Location URIs it returns.
func (c *Config) GetSbiUri() string {
	return c.GetSbiRegisterAddrs().URI(c.GetSbiScheme(), c.GetSbiPort(), c.GetSbiPreferredFamily())
}
