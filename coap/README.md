# Mainflux CoAP Adapter

Mainflux CoAP adapter provides an [CoAP](http://coap.technology/) API for sending messages through the
platform.

## Configuration

The service is configured using the environment variables presented in the
following table. Note that any unset variables will be replaced with their
default values.

| Variable              | Description            | Default                 |
|-----------------------|------------------------|-------------------------|
| MF_COAP_ADAPTER_PORT  | adapter listening port | `5683`                  |
| MF_NATS_URL           | NATS instance URL      | `nats://localhost:4222` |
| FM_MANAGER_URL        | manager service URL    | `http://localhost:8180` |

## Deployment

The service is distributed as Docker container. The following snippet provides
a compose file template that can be used to deploy the service container locally:

```yaml
version: "2"
services:
  adapter:
    image: mainflux/coap-adapter:[version]
    container_name: [instance name]
    ports:
      - [host machine port]:[configured port]
    environment:
      COAP_ADAPTER_NATS_URL: [NATS instance URL]
```

Running this service outside of container requires working instance of the NATS service.
To build the service outside of the container, execute the following shell script:

```bash
make coap
cd build
./mainflux-coap
```
