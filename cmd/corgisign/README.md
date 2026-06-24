# corgisign CLI

The official command-line client for the CorgiSign public API (`/api/v1`). It is
a small, dependency-free wrapper over the [Go SDK](../../) that authenticates
with an organisation API key.

## Install / build

```sh
# from sdk/go
make            # builds ./bin/corgisign
make install    # go install to $GOBIN (or $GOPATH/bin)

# or directly
go build -o corgisign ./cmd/corgisign
go install github.com/Corgi-Star/corgisign-go/cmd/corgisign@latest
```

## Configure

```sh
export CORGISIGN_API_KEY=cs_live_xxxxxxxx          # required — mint in the app under Settings → API keys
export CORGISIGN_BASE_URL=https://v2.sign.corgiinsure.com   # optional — defaults to this
```

`CORGISIGN_API_URL` is accepted as an alias for `CORGISIGN_BASE_URL`.

## Commands

```sh
corgisign whoami                       # which org / team / scopes does my key resolve to?
corgisign templates list               # reusable templates and their recipient roles
corgisign envelopes list               # newest first
corgisign envelopes list --status sent --limit 10
corgisign envelopes get <id>           # status + recipients
corgisign envelopes send <id>          # send a draft
corgisign send <id>                    # alias for "envelopes send"
corgisign version
```

Add `--json` to any command for the raw API JSON (handy for piping into `jq`).

### Create an envelope from a template

```sh
corgisign envelopes create \
  --template <templateId> \
  --title "Mutual NDA" \
  --recipient role=signer,name=Jane,email=jane@example.com \
  --send
```

### Create an envelope from an inline PDF

```sh
corgisign envelopes create \
  --document ./contract.pdf \
  --team <teamId> \
  --title "Contract" \
  --recipient name=Jane,email=jane@example.com \
  --send
```

`--recipient` and `--field` are repeatable. Recipient keys: `role`,
`placeholderId`, `name`, `email`, `signingOrder`. Field keys: `role`, `type`,
`value`.

## How auth works

Every request sends your key as `Authorization: Bearer <key>` (and `X-API-Key`)
to `/api/v1/*`, scoped by the server to the key's organisation, its team (if
pinned) and its capability scopes. Verify a key end to end with `corgisign whoami`.
