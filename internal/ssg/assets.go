package ssg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// assetDir is the slash-separated directory, within the output, that holds the
// assets. Every file in it is fingerprinted, so it can be cached forever.
const assetDir = "assets"

// stylesheet is the slash-separated path, within the assets, of the stylesheet
// that imports all the others.
const stylesheet = assetDir + "/site.css"

// hashLen is the number of hex digits of a file's SHA-256 kept in its name.
const hashLen = 10

// assetsKey is the context key under which Build stores the URLs of the
// fingerprinted assets.
type assetsKey struct{}

// Asset returns the URL of the asset at the URL path p, such as
// "/assets/site.css". Build gives every asset a name that changes with its
// contents, which lets browsers cache it forever, so pages must reach assets
// through Asset rather than by their source path.
//
// Asset reports an error for a path that is not an asset, so that a typo fails
// the build. Outside Build, as when a test renders a page directly, it returns p
// unchanged.
func Asset(ctx context.Context, p string) (string, error) {
	urls, ok := ctx.Value(assetsKey{}).(map[string]string)
	if !ok {
		return p, nil
	}
	u, ok := urls[p]
	if !ok {
		return "", fmt.Errorf("unknown asset %s", p)
	}
	return u, nil
}

// fingerprint renames every file under the asset directory of out to include a
// hash of its contents, bundling the stylesheets on the way, and returns the
// URL of each asset by its source URL.
//
// Stylesheets are hashed last, after their bundle has been rewritten to point
// at the fingerprinted files it references, such as fonts. Other files are
// hashed as they are, so a script must not import another by a relative path.
func fingerprint(out string) (map[string]string, error) {
	// esbuild reports absolute paths, which must be relative to out.
	out, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	urls := make(map[string]string)
	root := filepath.Join(out, assetDir)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return urls, nil
	}
	err = filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(name) == ".css" {
			return err
		}
		return rename(out, name, urls)
	})
	if err != nil {
		return nil, err
	}
	entry := filepath.Join(out, filepath.FromSlash(stylesheet))
	if _, err := os.Stat(entry); os.IsNotExist(err) {
		return urls, nil
	}
	if err := bundleCSS(out, entry, urls); err != nil {
		return nil, err
	}
	if err := rename(out, entry, urls); err != nil {
		return nil, err
	}
	return urls, nil
}

// rename moves the file name, within out, to a name with a hash of its
// contents before the extension, and records its new URL in urls.
func rename(out, name string, urls map[string]string) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	ext := filepath.Ext(name)
	hashed := strings.TrimSuffix(name, ext) + "." + hex.EncodeToString(sum[:])[:hashLen] + ext
	if err := os.Rename(name, hashed); err != nil {
		return err
	}
	from, err := urlPath(out, name)
	if err != nil {
		return err
	}
	to, err := urlPath(out, hashed)
	if err != nil {
		return err
	}
	urls[from] = to
	return nil
}

// urlPath returns the URL path at which the file name, within out, is served.
func urlPath(out, name string) (string, error) {
	rel, err := filepath.Rel(out, name)
	if err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(rel), nil
}

// bundleCSS replaces the stylesheet at entry with a minified bundle of itself
// and everything it imports, then removes the other stylesheets beside it,
// which the bundle has made redundant. The files the bundle references with
// url(), such as fonts, stay separate and are rewritten to their URLs in urls.
func bundleCSS(out, entry string, urls map[string]string) error {
	res := api.Build(api.BuildOptions{
		EntryPoints:      []string{entry},
		Outfile:          entry,
		Bundle:           true,
		MinifyWhitespace: true,
		MinifySyntax:     true,
		AllowOverwrite:   true,
		Write:            true,
		LogLevel:         api.LogLevelSilent,
		Plugins:          []api.Plugin{fingerprintedURLs(out, urls)},
	})
	if len(res.Errors) > 0 {
		msgs := api.FormatMessages(res.Errors, api.FormatMessagesOptions{})
		return fmt.Errorf("bundle css: %s", strings.Join(msgs, ""))
	}
	others, err := filepath.Glob(filepath.Join(filepath.Dir(entry), "*.css"))
	if err != nil {
		return err
	}
	for _, name := range others {
		if name == entry {
			continue
		}
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

// fingerprintedURLs is an esbuild plugin that leaves the files referenced with
// url() out of the bundle, pointing at their fingerprinted URLs instead.
func fingerprintedURLs(out string, urls map[string]string) api.Plugin {
	return api.Plugin{
		Name: "fingerprinted-urls",
		Setup: func(b api.PluginBuild) {
			b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				if args.Kind != api.ResolveCSSURLToken {
					return api.OnResolveResult{}, nil
				}
				if strings.Contains(args.Path, ":") {
					// A data: URI or an absolute URL is not an asset.
					return api.OnResolveResult{Path: args.Path, External: true}, nil
				}
				src := args.Path
				if !path.IsAbs(src) {
					importer, err := urlPath(out, args.Importer)
					if err != nil {
						return api.OnResolveResult{}, err
					}
					src = path.Join(path.Dir(importer), src)
				}
				u, ok := urls[src]
				if !ok {
					return api.OnResolveResult{}, fmt.Errorf("unknown asset %s", src)
				}
				return api.OnResolveResult{Path: u, External: true}, nil
			})
		},
	}
}
