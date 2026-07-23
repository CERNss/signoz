import deleteLocalStorageKey from 'api/browser/localstorage/remove';
import getSessionLogoutContext from 'api/v2/sessions/logout/get';
import { LOCALSTORAGE } from 'constants/localStorage';
import ROUTES from 'constants/routes';
import history from 'lib/history';

import deleteSession from './v2/sessions/delete';

export const Logout = async (): Promise<void> => {
	let providerLogoutURL = '';

	try {
		const response = await getSessionLogoutContext({ ref: window.location.href });
		providerLogoutURL = response?.data?.url || '';
	} catch (error) {
		console.error(error);
	}

	try {
		await deleteSession();
	} catch (error) {
		console.error(error);
	}

	deleteLocalStorageKey(LOCALSTORAGE.AUTH_TOKEN);
	deleteLocalStorageKey(LOCALSTORAGE.IS_LOGGED_IN);
	deleteLocalStorageKey(LOCALSTORAGE.IS_IDENTIFIED_USER);
	deleteLocalStorageKey(LOCALSTORAGE.REFRESH_AUTH_TOKEN);
	deleteLocalStorageKey(LOCALSTORAGE.LOGGED_IN_USER_EMAIL);
	deleteLocalStorageKey(LOCALSTORAGE.LOGGED_IN_USER_NAME);
	deleteLocalStorageKey(LOCALSTORAGE.CHAT_SUPPORT);
	deleteLocalStorageKey(LOCALSTORAGE.USER_ID);
	deleteLocalStorageKey(LOCALSTORAGE.QUICK_FILTERS_SETTINGS_ANNOUNCEMENT);
	window.dispatchEvent(new CustomEvent('LOGOUT'));

	if (providerLogoutURL) {
		// oxlint-disable-next-line signoz/no-raw-absolute-path -- provider end-session URL is external
		window.location.href = providerLogoutURL;
		return;
	}

	history.push(ROUTES.LOGIN);
};
