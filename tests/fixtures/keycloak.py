import time

import docker
import docker.errors
import pytest
import requests
from testcontainers.core.container import Network
from testcontainers.keycloak import KeycloakContainer

from fixtures import reuse, types
from fixtures.logger import setup_logger

logger = setup_logger(__name__)

IDP_ROOT_USERNAME = "admin"
IDP_ROOT_PASSWORD = "password"

# Keycloak needs a while before the management port serves /health/ready.
IDP_READY_TIMEOUT_SECONDS = 180.0
IDP_READY_POLL_SECONDS = 2.0


def _start_and_wait_until_ready(container: KeycloakContainer) -> None:
    """Start Keycloak, tolerating a readiness probe that gives up on a 404.

    testcontainers' KeycloakContainer.start() polls <management>/health/ready
    but only retries ConnectionError and ReadTimeout, so raise_for_status()
    turns the window where the management port is already bound while the
    health endpoint is not yet served into an immediate HTTPError. The
    container itself is fine and becomes ready a few seconds later, so poll
    for it here instead of failing the whole suite.
    """
    try:
        container.start()
        return
    except requests.exceptions.HTTPError as err:
        logger.info("keycloak readiness probe returned %s, polling instead", err)

    url = f"{container.get_management_url()}/health/ready"
    deadline = time.monotonic() + IDP_READY_TIMEOUT_SECONDS
    last_status: object = None
    while time.monotonic() < deadline:
        try:
            response = requests.get(url, timeout=5)
            if response.ok:
                return
            last_status = response.status_code
        except requests.exceptions.RequestException as err:
            last_status = type(err).__name__
        time.sleep(IDP_READY_POLL_SECONDS)

    raise TimeoutError(f"keycloak was not ready at {url}, last result: {last_status}")


@pytest.fixture(name="idp", scope="package")
def idp(
    network: Network,
    request: pytest.FixtureRequest,
    pytestconfig: pytest.Config,
) -> types.TestContainerIDP:

    def create() -> types.TestContainerIDP:
        container = KeycloakContainer(
            image="quay.io/keycloak/keycloak:26.3.0",
            username=IDP_ROOT_USERNAME,
            password=IDP_ROOT_PASSWORD,
            port=6060,
            management_port=6061,
        )
        container.with_env("KC_HTTP_PORT", "6060")
        container.with_env("KC_HTTP_MANAGEMENT_PORT", "6061")
        container.with_network(network)
        _start_and_wait_until_ready(container)

        return types.TestContainerIDP(
            container=types.TestContainerDocker(
                id=container.get_wrapped_container().id,
                host_configs={
                    "6060": types.TestContainerUrlConfig(
                        "http",
                        container.get_container_host_ip(),
                        container.get_exposed_port(6060),
                    ),
                    "6061": types.TestContainerUrlConfig(
                        "http",
                        container.get_container_host_ip(),
                        container.get_exposed_port(6061),
                    ),
                },
                container_configs={
                    "6060": types.TestContainerUrlConfig("http", container.get_wrapped_container().name, 6060),
                    "6061": types.TestContainerUrlConfig("http", container.get_wrapped_container().name, 6061),
                },
            ),
        )

    def delete(container: types.TestContainerIDP):
        client = docker.from_env()

        try:
            client.containers.get(container_id=container.container.id).stop()
            client.containers.get(container_id=container.container.id).remove(v=True)
        except docker.errors.NotFound:
            logger.info(
                "Skipping removal of IDP, IDP(%s) not found. Maybe it was manually removed?",
                {"id": container.container.id},
            )

    def restore(cache: dict) -> types.TestContainerIDP:
        container = types.TestContainerDocker.from_cache(cache["container"])
        return types.TestContainerIDP(
            container=container,
        )

    return reuse.wrap(
        request,
        pytestconfig,
        "idp",
        lambda: types.TestContainerIDP(container=types.TestContainerDocker(id="", host_configs={}, container_configs={})),
        create,
        delete,
        restore,
    )
