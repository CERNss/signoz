export interface PostableAuthDomain {
	name: string;
	config: Config;
}

export interface Config {
	ssoEnabled: boolean;
	ssoType: string;
	samlConfig?: SAMLConfig;
	googleAuthConfig?: GoogleAuthConfig;
	oidcConfig?: OIDCConfig;
}

export interface SAMLConfig {
	samlEntity: string;
	samlIdp: string;
	samlCert: string;
	insecureSkipAuthNRequestsSigned: boolean;
}

export interface GoogleAuthConfig {
	clientId: string;
	clientSecret: string;
	redirectURI: string;
}

export interface OIDCConfig {
	issuer: string;
	issuerAlias: string;
	clientId: string;
	clientSecret: string;
	scopes?: string[];
	claimMapping: ClaimMapping;
	emailVerifiedPolicy?: string;
	enforceEmailDomain?: boolean;
	allowJit?: boolean;
	insecureSkipEmailVerified: boolean;
	getUserInfo: boolean;
}

export interface ClaimMapping {
	email: string;
}
