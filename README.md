# govanity

Statically generate HTML for Go vanity imports.

The generated HTML is compatible with GitHub Pages.

## Install

`go get -u pack.ag/cmd/govanity`

## Usage

* Requires `go` and `git` on your `$PATH`.
* Packages must have an [import comment](https://golang.org/cmd/go/#hdr-Import_path_checking) matching the provided prefix.
* A shallow clone of every Go repository found is done into a temp directory. This may take some time depending on number 
  of repositories and their sizes. It would be more efficient to cache the repositories and pull them on each run, which
  may be implemented in the future.

```
govanity
Usage: govanity [flags]

Options can be provided via flags or environment variables.

  -cname
    	write CNAME file for GitHub Pages (default: false) [GOVANITY_CNAME]
  -out string
    	base directory to write generated files to (required) [GOVANITY_OUT]
  -prefix string
    	vanity URL prefix to match in import comments (required) [GOVANITY_PREFIX]
  -search string
    	comma seperated list of GitHub usernames/orgs/repos to search (required) [GOVANITY_SEARCH]
  -token string
    	GitHub API token to avoid rate limiting (optional) [GOVANITY_GITHUB_TOKEN]


Searching usernames/organizations requires multiple GitHub API calls. Rate limiting is likely to occur
without providing an API token.

Example:

> govanity -prefix=pack.ag -search="vcabbage/go-tftp,packag" -out "$HOME/src/packag.github.io" -cname=true

This will search the repository vcabbage/go-tftp and all repositories in the packag organization for Go packages
with an import comments beginning with "pack.ag" (ie, 'package tftp // import "pack.ag/tftp"'). Appropriate
HTML with <go-import> and <go-source> tags will be written to $HOME/src/packag.github.io.
```

## Issues/Contributions

I wrote this tool to make managing vanity imports easier for myself and it's therefor opinionated and limited in someways.
Contributions are welcome, but please open an issue before spending your time on code.

## Attribution

The HTML template was copied from [@natefinch](https://github.com/natefinch)'s [Vanity Imports with Hugo](https://npf.io/2016/10/vanity-imports-with-hugo/) post.
