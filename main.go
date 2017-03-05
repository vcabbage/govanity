package main // import "pack.ag/cmd/govanity"

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func configuration() (config, error) {
	cname := os.Getenv("GOVANITY_CNAME")
	cfg := config{
		prefix:      os.Getenv("GOVANITY_PREFIX"),
		search:      os.Getenv("GOVANITY_SEARCH"),
		out:         os.Getenv("GOVANITY_OUT"),
		githubToken: os.Getenv("GOVANITY_GITHUB_TOKEN"),
		writeCNAME:  cname != "" && cname != "0",
	}

	flag.StringVar(&cfg.prefix, "prefix", cfg.prefix, "vanity URL prefix to match in import comments (required) [GOVANITY_PREFIX]")
	flag.StringVar(&cfg.search, "search", cfg.search, "comma seperated list of GitHub usernames/orgs/repos to search (required) [GOVANITY_SEARCH]")
	flag.StringVar(&cfg.out, "out", cfg.out, "base directory to write generated files to (required) [GOVANITY_OUT]")
	flag.BoolVar(&cfg.writeCNAME, "cname", cfg.writeCNAME, "write CNAME file for GitHub Pages (default: false) [GOVANITY_CNAME]")
	flag.StringVar(&cfg.githubToken, "token", cfg.githubToken, "GitHub API token to avoid rate limiting (optional) [GOVANITY_GITHUB_TOKEN]")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: govanity [flags]

Options can be provided via flags or environment variables.

`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `

Searching usernames/organizations requires multiple GitHub API calls. Rate limiting is likely to occur
without providing an API token.

Example:

> govanity -prefix=pack.ag -search="vcabbage/go-tftp,packag" -out "$HOME/src/packag.github.io" -cname=true

This will search the repository vcabbage/go-tftp and all repositories in the packag organization for Go packages
with an import comments beginning with "pack.ag" (ie, 'package tftp // import "pack.ag/tftp"'). Appropriate
HTML with <go-import> and <go-source> tags will be written to $HOME/src/packag.github.io.
`)
	}

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(2)
	}

	flag.Parse()

	err := cfg.Parse()

	return cfg, err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := configuration()
	if err != nil {
		return err
	}

	fmt.Printf("Prefix=%q Search List=%+v Out=%q Token=%t Write CNAME=%t\n", cfg.prefix, cfg.searchList, cfg.out, cfg.githubToken != "", cfg.writeCNAME)

	ctx := context.Background()

	var client *http.Client
	if cfg.githubToken != "" {
		client = oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.githubToken}))
	}
	gh := github.NewClient(client)

	repoURLs, err := getPotentialRepos(ctx, gh, cfg.searchList)
	if err != nil {
		return err
	}

	var imports []vanityImport
	for _, repo := range repoURLs {
		fmt.Printf("Pulling %s\n", repo)
		packages, err := getVanityPackages(ctx, repo, cfg.prefix)
		if err != nil {
			fmt.Printf("\t%v\n", err)
			continue
		}

		for _, pkg := range packages {
			fmt.Printf("Found match: %s -> %s\n", pkg.Import, pkg.RepoURL)
		}
		fmt.Printf("Found %d matching packages.\n", len(packages))

		imports = append(imports, packages...)
	}

	for _, imprt := range imports {
		htmlPath := imprt.htmlPath(cfg.prefix, cfg.out)
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		f, err := os.Create(htmlPath)
		if err != nil {
			fmt.Printf("Error creating %s: %v\n", htmlPath, err)
			continue
		}
		defer f.Close()

		if err := tmpl.Execute(f, imprt); err != nil {
			fmt.Printf("Error writing %s: %v\n", htmlPath, err)
			continue
		}
	}

	if cfg.writeCNAME {
		err := ioutil.WriteFile(filepath.Join(cfg.out, "CNAME"), []byte(cfg.prefixURL.Host+"\n"), 0644)
		if err != nil {
			return fmt.Errorf("writing CNAME file: %v", err)
		}
	}

	return nil
}

type config struct {
	prefix      string
	prefixURL   *url.URL
	search      string
	searchList  []string
	out         string
	githubToken string
	writeCNAME  bool
}

func (cfg *config) Parse() error {
	if cfg.prefix == "" {
		return errors.New("must provide vanity URL prefix")
	}

	u, err := url.Parse("//" + cfg.prefix)
	if err != nil {
		return fmt.Errorf("invalid URL (%v)", err)
	}
	cfg.prefixURL = u

	if cfg.search == "" {
		return errors.New("search list must contain at least one entry")
	}

	for _, search := range strings.Split(cfg.search, ",") {
		search = strings.TrimSpace(search)
		if search != "" {
			cfg.searchList = append(cfg.searchList, search)
		}
	}
	return nil
}

func getPotentialRepos(ctx context.Context, gh *github.Client, search []string) (repoURLs []string, _ error) {
	// Pull out repos and make a map for dup check
	searchRepos := make(map[string]struct{})
	var usernames []string
	for _, v := range search {
		if !strings.ContainsRune(v, '/') {
			usernames = append(usernames, v)
			continue
		}

		repoURLs = append(repoURLs, "https://github.com/"+v)
		searchRepos[v] = struct{}{}
	}

	for _, username := range usernames {
		repos, _, err := gh.Repositories.List(ctx, username, nil)
		if err != nil {
			fmt.Printf("%s: %v", username, err)
			continue
		}

		for _, repo := range repos {
			repoName := repo.GetName()

			if _, ok := searchRepos[username+"/"+repoName]; ok {
				fmt.Printf("%s/%s: is explicitly listed\n", username, repoName)
				continue
			}

			if repo.GetFork() {
				fmt.Printf("%s/%s: is a fork\n", username, repoName)
				continue
			}

			if repo.GetLanguage() == "Go" {
				repoURLs = append(repoURLs, repo.GetSVNURL())
				continue
			}

			languages, _, err := gh.Repositories.ListLanguages(ctx, username, repoName)
			if err != nil {
				fmt.Printf("%s: %v", username, err)
				continue
			}
			if _, ok := languages["Go"]; !ok {
				fmt.Printf("%s/%s: not a Go repository\n", username, repoName)
				continue
			}

			repoURLs = append(repoURLs, repo.GetSVNURL())
		}
	}
	return repoURLs, nil
}

func getVanityPackages(ctx context.Context, url, base string) ([]vanityImport, error) {
	var imports []vanityImport

	tmpDir, err := ioutil.TempDir("", "govanity")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", url, tmpDir)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	cmd = exec.CommandContext(ctx, "go", "list", "-f={{.ImportComment}}:{{.Dir}}", "./...")
	cmd.Dir = tmpDir
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(out)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, base) {
			continue
		}

		s := strings.Split(line, ":")
		importPath := s[0]
		dir, err := filepath.EvalSymlinks(s[1])
		if err != nil {
			return nil, err
		}

		pathLen := 0
		if dir != tmpDir {
			dir = filepath.ToSlash(strings.TrimLeft(strings.TrimPrefix(dir, tmpDir), "/\\"))
			pathLen = len(strings.Split(dir, "/"))
		}

		imports = append(imports, vanityImport{
			Import:  importPath,
			RepoURL: url,
			pathLen: pathLen,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return imports, nil
}

type vanityImport struct {
	Import  string
	RepoURL string
	pathLen int
}

func (i vanityImport) ImportPrefix() string {
	importURL, err := url.Parse(i.Import)
	if err != nil {
		return ""
	}

	importPathSegments := strings.Split(importURL.Path, "/")
	importURL.Path = strings.Join(importPathSegments[:len(importPathSegments)-i.pathLen], "/")

	return importURL.String()
}

func (i vanityImport) htmlPath(base, dir string) string {
	return filepath.Join(dir, strings.TrimPrefix(i.Import, base)) + ".html"
}

var tmpl = template.Must(template.New("tmpl").Parse(`<!DOCTYPE html>
<head>
  <meta http-equiv="content-type" content="text/html; charset=utf-8">
  <meta name="go-import" content="{{.ImportPrefix}} git {{.RepoURL}}">
  <meta name="go-source" content="{{.ImportPrefix}} {{.RepoURL}} {{.RepoURL}}/tree/master{/dir} {{.RepoURL}}/blob/master{/dir}/{file}#L{line}">
  <meta http-equiv="refresh" content="0; url={{.RepoURL}}">
</head>
</html>
`))
