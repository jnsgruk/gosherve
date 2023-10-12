# gosherve

> A simple HTTP file server with some basic URL shortening/redirect functionality

This project is a simple web server written in Go that will:

- Serve files from a specified directory
- Serve redirects specified in a file hosted at a URL
- Report some metrics about the redirects served

I wrote this to satisfy the use case of hosting my very simple [personal website](https://jnsgr.uk), while also giving me the ability to set up custom short URLs relatively easily. I don't need any tracking, stats or clever features...

I host the "redirects file" in a Github Gist; it looks something like this:

```
github https://github.com/jnsgruk
linkedin https://linkedin.com/in/jnsgruk
something https://somelink.com
wow https://www.ohmygoodness.com
```

With this simple config, visiting [https://jnsgr.uk/linkedin](https://jnsgr.uk/linkedin) returns a `302` that redirects you to my LinkedIn page, etc. If an unknown URL is requested, the server first refreshes its list of redirects from the specified URL, and then either returns the redirect or a 404. There is some **very basic** parsing done on the redirects file to ensure entries are valid.

If file serving is enabled, the web server will always try to find a matching file before checking for a redirect.

## Configuration

The server is configured with two environment variables:

| Variable Name               |   Type   | Notes                                                                                           |
| :-------------------------- | :------: | :---------------------------------------------------------------------------------------------- |
| `GOSHERVE_WEBROOT`          | `string` | Path to directory from which to serve files. If not specified, file serving is simply disabled. |
| `GOSHERVE_REDIRECT_MAP_URL` | `string` | URL containing a list of aliases and corresponding redirect URLs                                |
| `GOSHERVE_LOG_LEVEL`        | `string` | Sets the log level. One of: `info`, `debug`, `warn`, `error`                                    |

## Hacking

The application has minimal dependencies and can be run like so:

```bash
git clone https://github.com/jnsgruk/gosherve

# Export some variables to configure gosherve
export GOSHERVE_REDIRECT_MAP_URL="https://gist.githubusercontent.com/someuser/somegisthash/raw"
export GOSHERVE_WEBROOT="/path/to/some/files"

# Run it!
go run ./cmd/gosherve/main.go
```

## Build & Release

This project uses goreleaser to manage builds and releases.

In local development, you can build a snapshot release like so:

```shell
goreleaser --snapshot --rm-dist
```

The output will be present in `dist/`.

To create a release, create a new tag and push to Github, the release will be automatically
created by Goreleaser.
