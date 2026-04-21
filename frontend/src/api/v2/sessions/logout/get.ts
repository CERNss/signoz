import { ApiV2Instance as axios } from 'api';
import { ErrorResponseHandlerV2 } from 'api/ErrorResponseHandlerV2';
import { AxiosError } from 'axios';
import { ErrorV2Resp, RawSuccessResponse, SuccessResponseV2 } from 'types/api';
import { Props, SessionLogoutContext } from 'types/api/v2/sessions/logout/get';

const get = async (
	props: Props,
): Promise<SuccessResponseV2<SessionLogoutContext>> => {
	try {
		const response = await axios.get<RawSuccessResponse<SessionLogoutContext>>(
			'/sessions/logout_context',
			{
				params: props,
			},
		);

		return {
			httpStatusCode: response.status,
			data: response.data.data,
		};
	} catch (error) {
		ErrorResponseHandlerV2(error as AxiosError<ErrorV2Resp>);
	}
};

export default get;
