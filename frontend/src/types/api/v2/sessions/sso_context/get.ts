export interface Props {
	ref: string;
}

export interface SSODomainContext {
	domain: string;
	provider: string;
	url: string;
}

export interface SessionSSOContext {
	domains: SSODomainContext[];
}
