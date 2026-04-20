ARG ALPINE_SHA="pass-a-valid-docker-sha-otherwise-this-will-fail"

FROM alpine@sha256:${ALPINE_SHA}
LABEL maintainer="signoz"
WORKDIR /root

ARG OS="linux"
ARG TARGETARCH

RUN apk update && \
	apk add ca-certificates && \
	rm -rf /var/cache/apk/*


COPY ./target/${OS}-${TARGETARCH}/signoz-community /root/signoz
COPY ./templates/email /root/templates
COPY frontend/build/ /etc/signoz/web/

RUN chmod 755 /root /root/signoz

ENTRYPOINT ["./signoz", "server"]
