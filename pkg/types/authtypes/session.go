package authtypes

import (
	"github.com/SigNoz/signoz/pkg/errors"
	"github.com/SigNoz/signoz/pkg/valuer"
)

type SessionContext struct {
	Exists bool                 `json:"exists"`
	Orgs   []*OrgSessionContext `json:"orgs"`
}

type SessionSSOContext struct {
	Domains []SSODomainContext `json:"domains"`
}

type SessionLogoutContext struct {
	URL string `json:"url"`
}

type SSODomainContext struct {
	Domain   string        `json:"domain"`
	Provider AuthNProvider `json:"provider"`
	URL      string        `json:"url"`
}

type OrgSessionContext struct {
	ID           valuer.UUID  `json:"id"`
	Name         string       `json:"name"`
	AuthNSupport AuthNSupport `json:"authNSupport"`
	Warning      *errors.JSON `json:"warning,omitempty"`
}

type AuthNSupport struct {
	Callback []CallbackAuthNSupport `json:"callback"`
	Password []PasswordAuthNSupport `json:"password"`
}

type CallbackAuthNSupport struct {
	Provider AuthNProvider `json:"provider"`
	URL      string        `json:"url"`
}

type PasswordAuthNSupport struct {
	Provider AuthNProvider `json:"provider"`
}

func NewSessionContext() *SessionContext {
	return &SessionContext{Exists: false, Orgs: []*OrgSessionContext{}}
}

func NewSessionSSOContext() *SessionSSOContext {
	return &SessionSSOContext{Domains: []SSODomainContext{}}
}

func NewSessionLogoutContext(url string) *SessionLogoutContext {
	return &SessionLogoutContext{URL: url}
}

func NewOrgSessionContext(orgID valuer.UUID, name string) *OrgSessionContext {
	return &OrgSessionContext{
		ID:   orgID,
		Name: name,
		AuthNSupport: AuthNSupport{
			Password: []PasswordAuthNSupport{},
			Callback: []CallbackAuthNSupport{},
		},
		Warning: nil,
	}
}

func (s *SessionContext) AddOrgContext(orgContext *OrgSessionContext) *SessionContext {
	s.Orgs = append(s.Orgs, orgContext)
	return s
}

func (s *SessionSSOContext) AddSSODomainContext(domainContext SSODomainContext) *SessionSSOContext {
	s.Domains = append(s.Domains, domainContext)
	return s
}

func NewSSODomainContext(domain string, provider AuthNProvider, url string) SSODomainContext {
	return SSODomainContext{
		Domain:   domain,
		Provider: provider,
		URL:      url,
	}
}

func (s *OrgSessionContext) AddPasswordAuthNSupport(provider AuthNProvider) *OrgSessionContext {
	s.AuthNSupport.Password = append(s.AuthNSupport.Password, PasswordAuthNSupport{Provider: provider})
	return s
}

func (s *OrgSessionContext) AddCallbackAuthNSupport(provider AuthNProvider, url string) *OrgSessionContext {
	s.AuthNSupport.Callback = append(s.AuthNSupport.Callback, CallbackAuthNSupport{Provider: provider, URL: url})
	return s
}

func (s *OrgSessionContext) AddWarning(warning error) *OrgSessionContext {
	s.Warning = errors.AsJSON(warning)
	return s
}
