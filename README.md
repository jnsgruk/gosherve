# gosherve

> A simple HTTP file server with some basic URL shortening/redirect functionality

This project is a simple web server written in Go that will:

- Serve files from a specified directory
- Serve redirects specified in a file hosted at a URL

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

| Variable Name      |   Type   | Notes                                                                                           |
| :----------------- | :------: | :---------------------------------------------------------------------------------------------- |
| `WEBROOT`          | `string` | Path to directory from which to serve files. If not specified, file serving is simply disabled. |
| `REDIRECT_MAP_URL` | `string` | URL containing a list of aliases and corresponding redirect URLs                                |

## Getting Started

The application has minimal dependencies and can be run like so:

```bash
# Clone this repo
mkdir -p $GOPATH/src/github.com/jnsgruk/gosherve
git clone https://github.com/jnsgruk/gosherve $GOPATH/src/github.com/jnsgruk/gosherve

# Export some variables to configure gosherve
export REDIRECT_MAP_URL="https://gist.githubusercontent.com/someuser/somegisthash/raw"
export WEBROOT="/path/to/some/files"

# Run it!
go run main.go
```

The application can be built with `go build -o gosherve main.go`
