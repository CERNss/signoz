FROM alpine:3.20.3
LABEL maintainer="signoz"
WORKDIR /root

ARG OS="linux"
ARG TARGETARCH

RUN set -eu; \
	attempt=1; \
	max=3; \
	until apk add --no-cache ca-certificates-bundle; do \
		if [ "${attempt}" -ge "${max}" ]; then \
			echo "apk add failed after ${max} attempts"; \
			exit 1; \
		fi; \
		sleep_time=$((attempt * 5)); \
		echo "apk add failed (attempt ${attempt}/${max}), retrying in ${sleep_time}s..."; \
		sleep "${sleep_time}"; \
		attempt=$((attempt + 1)); \
	done


COPY ./target/${OS}-${TARGETARCH}/signoz /root/signoz
COPY ./templates/email /root/templates
COPY frontend/build/ /etc/signoz/web/

RUN chmod 755 /root /root/signoz

ENTRYPOINT ["./signoz", "server"]
