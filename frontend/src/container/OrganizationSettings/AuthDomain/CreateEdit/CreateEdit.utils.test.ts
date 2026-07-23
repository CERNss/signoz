import {
	AuthtypesAuthDomainConfigGoogleDTOKind,
	AuthtypesAuthDomainConfigOIDCDTOKind,
	AuthtypesAuthDomainConfigSAMLDTOKind,
	AuthtypesAuthNProviderDTO,
} from 'api/generated/services/sigNoz.schemas';

import {
	convertDomainMappingsToList,
	convertDomainMappingsToRecord,
	convertGroupMappingsToList,
	convertGroupMappingsToRecord,
	convertScopesArrayToString,
	convertScopesStringToArray,
	prepareConfig,
	prepareInitialValues,
	prepareOIDCConfig,
} from './CreateEdit.utils';

describe('convertGroupMappingsToRecord', () => {
	it('returns undefined for an empty list', () => {
		expect(convertGroupMappingsToRecord([])).toBeUndefined();
	});

	it('returns undefined when input is undefined', () => {
		expect(convertGroupMappingsToRecord(undefined)).toBeUndefined();
	});

	it('converts entries to a Record', () => {
		expect(
			convertGroupMappingsToRecord([
				{ groupName: 'admins', role: 'ADMIN' },
				{ groupName: 'viewers', role: 'VIEWER' },
			]),
		).toStrictEqual({ admins: 'ADMIN', viewers: 'VIEWER' });
	});

	it('skips entries with missing groupName or role', () => {
		expect(
			convertGroupMappingsToRecord([
				{ groupName: 'admins', role: 'ADMIN' },
				{ groupName: '', role: 'VIEWER' },
				{ role: 'EDITOR' },
			]),
		).toStrictEqual({ admins: 'ADMIN' });
	});
});

describe('convertDomainMappingsToRecord', () => {
	it('returns undefined for an empty list', () => {
		expect(convertDomainMappingsToRecord([])).toBeUndefined();
	});

	it('returns undefined when input is undefined', () => {
		expect(convertDomainMappingsToRecord(undefined)).toBeUndefined();
	});

	it('converts entries to a Record', () => {
		expect(
			convertDomainMappingsToRecord([
				{ domain: 'example.com', adminEmail: 'admin@example.com' },
				{ domain: 'corp.io', adminEmail: 'it@corp.io' },
			]),
		).toStrictEqual({
			'example.com': 'admin@example.com',
			'corp.io': 'it@corp.io',
		});
	});
});

describe('round-trip fidelity', () => {
	it('Record → list → Record preserves group mappings', () => {
		const original = { admins: 'ADMIN', devs: 'EDITOR', viewers: 'VIEWER' };
		expect(
			convertGroupMappingsToRecord(convertGroupMappingsToList(original)),
		).toStrictEqual(original);
	});

	it('Record → list → Record preserves domain mappings', () => {
		const original = {
			'example.com': 'admin@example.com',
			'corp.io': 'it@corp.io',
		};
		expect(
			convertDomainMappingsToRecord(convertDomainMappingsToList(original)),
		).toStrictEqual(original);
	});
});

describe('prepareInitialValues', () => {
	it('returns empty defaults when no record is provided', () => {
		expect(prepareInitialValues(undefined)).toStrictEqual({
			name: '',
			enabled: false,
		});
	});

	it('hydrates groupMappings Record into groupMappingsList for the form', () => {
		const result = prepareInitialValues({
			id: 'domain-1',
			name: 'example.com',
			enabled: true,
			config: {
				kind: AuthtypesAuthDomainConfigSAMLDTOKind.saml,
				spec: {
					location: 'https://idp.example.com/sso',
					entityId: 'urn:example:idp',
					certificate: 'CERT',
				},
			},
			roleMapping: {
				defaultRole: 'VIEWER',
				useRoleAttribute: false,
				groupMappings: { admins: 'ADMIN', viewers: 'VIEWER' },
			},
		});

		expect(result.roleMapping?.groupMappingsList).toStrictEqual([
			{ groupName: 'admins', role: 'ADMIN' },
			{ groupName: 'viewers', role: 'VIEWER' },
		]);
	});

	it('hydrates domainToAdminEmail Record into domainToAdminEmailList for the form', () => {
		const result = prepareInitialValues({
			id: 'domain-1',
			name: 'example.com',
			enabled: true,
			config: {
				kind: AuthtypesAuthDomainConfigGoogleDTOKind.google,
				spec: {
					clientId: 'id',
					clientSecret: 'secret',
					domainToAdminEmail: { 'example.com': 'admin@example.com' },
				},
			},
		});

		expect(result.googleAuthConfig?.domainToAdminEmailList).toStrictEqual([
			{ domain: 'example.com', adminEmail: 'admin@example.com' },
		]);
	});

	it('sets groupMappingsList to empty array when roleMapping has no groupMappings', () => {
		const result = prepareInitialValues({
			id: 'domain-1',
			name: 'example.com',
			enabled: true,
			config: {
				kind: AuthtypesAuthDomainConfigOIDCDTOKind.oidc,
				spec: {
					issuer: 'https://oidc.example.com',
					clientId: 'id',
					clientSecret: 'secret',
				},
			},
			roleMapping: { defaultRole: 'VIEWER', useRoleAttribute: true },
		});

		expect(result.roleMapping?.groupMappingsList).toStrictEqual([]);
	});
});

describe('OIDC scopes serialization', () => {
	it('splits a comma or whitespace separated list into scopes', () => {
		expect(convertScopesStringToArray('openid, profile  email')).toStrictEqual([
			'openid',
			'profile',
			'email',
		]);
	});

	it('returns undefined for empty text', () => {
		expect(convertScopesStringToArray('')).toBeUndefined();
		expect(convertScopesStringToArray(undefined)).toBeUndefined();
	});

	it('joins scopes back into editable text', () => {
		expect(convertScopesArrayToString(['openid', 'groups'])).toBe(
			'openid, groups',
		);
		expect(convertScopesArrayToString(undefined)).toBe('');
	});
});

describe('prepareOIDCConfig', () => {
	it('serializes scopesText into a scopes array and drops the text field', () => {
		expect(
			prepareOIDCConfig({
				oidcConfig: {
					issuer: 'https://oidc.example.com',
					clientId: 'id',
					clientSecret: 'secret',
					scopesText: 'openid, groups',
				},
			}),
		).toStrictEqual({
			issuer: 'https://oidc.example.com',
			clientId: 'id',
			clientSecret: 'secret',
			scopes: ['openid', 'groups'],
		});
	});

	it('omits scopes when the text is empty', () => {
		expect(
			prepareOIDCConfig({
				oidcConfig: {
					issuer: 'https://oidc.example.com',
					clientId: 'id',
					clientSecret: 'secret',
					scopesText: '',
				},
			}),
		).toStrictEqual({
			issuer: 'https://oidc.example.com',
			clientId: 'id',
			clientSecret: 'secret',
		});
	});

	it('carries the OIDC policy fields into the config envelope', () => {
		expect(
			prepareConfig(
				{
					oidcConfig: {
						issuer: 'https://oidc.example.com',
						clientId: 'id',
						clientSecret: 'secret',
						scopesText: 'openid',
						allowJit: false,
						emailVerifiedPolicy: 'strict',
						enforceEmailDomain: true,
					},
				},
				AuthtypesAuthNProviderDTO.oidc,
			),
		).toStrictEqual({
			kind: AuthtypesAuthDomainConfigOIDCDTOKind.oidc,
			spec: {
				issuer: 'https://oidc.example.com',
				clientId: 'id',
				clientSecret: 'secret',
				scopes: ['openid'],
				allowJit: false,
				emailVerifiedPolicy: 'strict',
				enforceEmailDomain: true,
			},
		});
	});
});

describe('prepareInitialValues for OIDC', () => {
	it('hydrates scopesText and defaults allowJit to true', () => {
		const result = prepareInitialValues({
			id: 'domain-1',
			name: 'example.com',
			enabled: true,
			config: {
				kind: AuthtypesAuthDomainConfigOIDCDTOKind.oidc,
				spec: {
					issuer: 'https://oidc.example.com',
					clientId: 'id',
					clientSecret: 'secret',
					scopes: ['openid', 'groups'],
				},
			},
		});

		expect(result.oidcConfig?.scopesText).toBe('openid, groups');
		expect(result.oidcConfig?.allowJit).toBe(true);
	});

	it('keeps an explicit allowJit of false', () => {
		const result = prepareInitialValues({
			id: 'domain-1',
			name: 'example.com',
			enabled: true,
			config: {
				kind: AuthtypesAuthDomainConfigOIDCDTOKind.oidc,
				spec: {
					issuer: 'https://oidc.example.com',
					clientId: 'id',
					clientSecret: 'secret',
					allowJit: false,
				},
			},
		});

		expect(result.oidcConfig?.allowJit).toBe(false);
		expect(result.oidcConfig?.scopesText).toBe('');
	});
});
