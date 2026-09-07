# Building Chat

Run from the repository root with Go 1.26+ and Mage:

```sh
mage
mage buildOnly "chat-api-chat check-component"
CGO_ENABLED=0 PLATFORMS=linux_amd64 mage
mage pack
```

Linux services are written to `_output/bin/platforms/linux/amd64/chat-*`;
tools are under `_output/bin/tools/linux/amd64/`.
The portable runtime archive is `_output/chat-linux-amd64.zip`.
It includes a compiled Linux/amd64 `mage` launcher, Mage startup files, config
and module metadata, not the full source. Use `./mage start`, `./mage stop` and
`./mage check` in the extracted release without installing Go or downloading
Go dependencies on the deployment host.

## Local images

```sh
docker build -t chat:local .
# Optional service-specific default command, using the image just built:
docker build -f build/images/chat-rpc-chat/Dockerfile -t chat-rpc-chat:local .
```

All service recipes derive from the locally built `chat:local` image.
