/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gogit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/fluxcd/pkg/gitutil"
	"github.com/fluxcd/pkg/version"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	git2 "github.com/fluxcd/source-controller/internal/git"
	"github.com/fluxcd/source-controller/pkg/git"
	extgogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

func CheckoutStrategyForRef(ref *sourcev1.GitRepositoryRef, opt git.CheckoutOptions) git.CheckoutStrategy {
	switch {
	case ref == nil:
		return &CheckoutBranch{branch: git.DefaultBranch, depth: opt.Depth}
	case ref.SemVer != "":
		return &CheckoutSemVer{semVer: ref.SemVer, recurseSubmodules: opt.RecurseSubmodules, depth: opt.Depth}
	case ref.Tag != "":
		return &CheckoutTag{tag: ref.Tag, recurseSubmodules: opt.RecurseSubmodules, depth: opt.Depth}
	case ref.Commit != "":
		strategy := &CheckoutCommit{branch: ref.Branch, commit: ref.Commit, recurseSubmodules: opt.RecurseSubmodules, depth: opt.Depth}
		if strategy.branch == "" {
			strategy.branch = git.DefaultBranch
		}
		return strategy
	case ref.Branch != "":
		return &CheckoutBranch{branch: ref.Branch, recurseSubmodules: opt.RecurseSubmodules, depth: opt.Depth}
	default:
		return &CheckoutBranch{branch: git.DefaultBranch, depth: opt.Depth}
	}
}

type CheckoutBranch struct {
	repo              *extgogit.Repository
	branch            string
	recurseSubmodules bool
	depth             int
}

func (c *CheckoutBranch) Checkout(ctx context.Context, path, url string, auth *git.Auth) (git.Commit, string, error) {
	repo, err := extgogit.PlainCloneContext(ctx, path, false, &extgogit.CloneOptions{
		URL:               url,
		Auth:              auth.AuthMethod,
		RemoteName:        git.DefaultOrigin,
		ReferenceName:     plumbing.NewBranchReferenceName(c.branch),
		SingleBranch:      true,
		NoCheckout:        false,
		Depth:             c.depth,
		RecurseSubmodules: recurseSubmodules(c.recurseSubmodules),
		Progress:          nil,
		Tags:              extgogit.NoTags,
		CABundle:          auth.CABundle,
	})
	if err != nil {
		return nil, "", fmt.Errorf("unable to clone '%s', error: %w", url, gitutil.GoGitError(err))
	}
	c.repo = repo
	head, err := repo.Head()
	if err != nil {
		return nil, "", fmt.Errorf("git resolve HEAD error: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, "", fmt.Errorf("git commit '%s' not found: %w", head.Hash(), err)
	}
	return &Commit{commit}, fmt.Sprintf("%s/%s", c.branch, head.Hash().String()), nil
}

func (c *CheckoutBranch) ExistsTargetFilesInRecentlyCommits(startHash, endHash, ignore string) (bool, error) {
	return existsTargetFilesInRecentlyCommits(c.repo, startHash, endHash, ignore)
}

type CheckoutTag struct {
	repo              *extgogit.Repository
	tag               string
	recurseSubmodules bool
	depth             int
}

func (c *CheckoutTag) Checkout(ctx context.Context, path, url string, auth *git.Auth) (git.Commit, string, error) {
	repo, err := extgogit.PlainCloneContext(ctx, path, false, &extgogit.CloneOptions{
		URL:               url,
		Auth:              auth.AuthMethod,
		RemoteName:        git.DefaultOrigin,
		ReferenceName:     plumbing.NewTagReferenceName(c.tag),
		SingleBranch:      true,
		NoCheckout:        false,
		Depth:             c.depth,
		RecurseSubmodules: recurseSubmodules(c.recurseSubmodules),
		Progress:          nil,
		Tags:              extgogit.NoTags,
		CABundle:          auth.CABundle,
	})
	if err != nil {
		return nil, "", fmt.Errorf("unable to clone '%s', error: %w", url, err)
	}
	c.repo = repo
	head, err := repo.Head()
	if err != nil {
		return nil, "", fmt.Errorf("git resolve HEAD error: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, "", fmt.Errorf("git commit '%s' not found: %w", head.Hash(), err)
	}
	return &Commit{commit}, fmt.Sprintf("%s/%s", c.tag, head.Hash().String()), nil
}

func (c *CheckoutTag) ExistsTargetFilesInRecentlyCommits(startHash, endHash, ignore string) (bool, error) {
	return existsTargetFilesInRecentlyCommits(c.repo, startHash, endHash, ignore)
}

type CheckoutCommit struct {
	repo              *extgogit.Repository
	branch            string
	commit            string
	recurseSubmodules bool
	depth             int
}

func (c *CheckoutCommit) Checkout(ctx context.Context, path, url string, auth *git.Auth) (git.Commit, string, error) {
	repo, err := extgogit.PlainCloneContext(ctx, path, false, &extgogit.CloneOptions{
		URL:               url,
		Auth:              auth.AuthMethod,
		RemoteName:        git.DefaultOrigin,
		ReferenceName:     plumbing.NewBranchReferenceName(c.branch),
		SingleBranch:      true,
		NoCheckout:        false,
		Depth:             c.depth,
		RecurseSubmodules: recurseSubmodules(c.recurseSubmodules),
		Progress:          nil,
		Tags:              extgogit.NoTags,
		CABundle:          auth.CABundle,
	})
	if err != nil {
		return nil, "", fmt.Errorf("unable to clone '%s', error: %w", url, err)
	}
	c.repo = repo
	w, err := repo.Worktree()
	if err != nil {
		return nil, "", fmt.Errorf("git worktree error: %w", err)
	}
	commit, err := repo.CommitObject(plumbing.NewHash(c.commit))
	if err != nil {
		return nil, "", fmt.Errorf("git commit '%s' not found: %w", c.commit, err)
	}
	err = w.Checkout(&extgogit.CheckoutOptions{
		Hash:  commit.Hash,
		Force: true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("git checkout error: %w", err)
	}
	return &Commit{commit}, fmt.Sprintf("%s/%s", c.branch, commit.Hash.String()), nil
}

func (c *CheckoutCommit) ExistsTargetFilesInRecentlyCommits(startHash, endHash, ignore string) (bool, error) {
	return existsTargetFilesInRecentlyCommits(c.repo, startHash, endHash, ignore)
}

type CheckoutSemVer struct {
	repo              *extgogit.Repository
	semVer            string
	recurseSubmodules bool
	depth             int
}

func (c *CheckoutSemVer) Checkout(ctx context.Context, path, url string, auth *git.Auth) (git.Commit, string, error) {
	verConstraint, err := semver.NewConstraint(c.semVer)
	if err != nil {
		return nil, "", fmt.Errorf("semver parse range error: %w", err)
	}

	repo, err := extgogit.PlainCloneContext(ctx, path, false, &extgogit.CloneOptions{
		URL:               url,
		Auth:              auth.AuthMethod,
		RemoteName:        git.DefaultOrigin,
		NoCheckout:        false,
		Depth:             c.depth,
		RecurseSubmodules: recurseSubmodules(c.recurseSubmodules),
		Progress:          nil,
		Tags:              extgogit.AllTags,
		CABundle:          auth.CABundle,
	})
	if err != nil {
		return nil, "", fmt.Errorf("unable to clone '%s', error: %w", url, err)
	}
	c.repo = repo

	repoTags, err := repo.Tags()
	if err != nil {
		return nil, "", fmt.Errorf("git list tags error: %w", err)
	}

	tags := make(map[string]string)
	tagTimestamps := make(map[string]time.Time)
	_ = repoTags.ForEach(func(t *plumbing.Reference) error {
		revision := plumbing.Revision(t.Name().String())
		hash, err := repo.ResolveRevision(revision)
		if err != nil {
			return fmt.Errorf("unable to resolve tag revision: %w", err)
		}
		commit, err := repo.CommitObject(*hash)
		if err != nil {
			return fmt.Errorf("unable to resolve commit of a tag revision: %w", err)
		}
		tagTimestamps[t.Name().Short()] = commit.Committer.When

		tags[t.Name().Short()] = t.Strings()[1]
		return nil
	})

	var matchedVersions semver.Collection
	for tag := range tags {
		v, err := version.ParseVersion(tag)
		if err != nil {
			continue
		}
		if !verConstraint.Check(v) {
			continue
		}
		matchedVersions = append(matchedVersions, v)
	}
	if len(matchedVersions) == 0 {
		return nil, "", fmt.Errorf("no match found for semver: %s", c.semVer)
	}

	// Sort versions
	sort.SliceStable(matchedVersions, func(i, j int) bool {
		left := matchedVersions[i]
		right := matchedVersions[j]

		if !left.Equal(right) {
			return left.LessThan(right)
		}

		// Having tag target timestamps at our disposal, we further try to sort
		// versions into a chronological order. This is especially important for
		// versions that differ only by build metadata, because it is not considered
		// a part of the comparable version in Semver
		return tagTimestamps[left.String()].Before(tagTimestamps[right.String()])
	})
	v := matchedVersions[len(matchedVersions)-1]
	t := v.Original()

	w, err := repo.Worktree()
	if err != nil {
		return nil, "", fmt.Errorf("git worktree error: %w", err)
	}

	err = w.Checkout(&extgogit.CheckoutOptions{
		Branch: plumbing.NewTagReferenceName(t),
	})
	if err != nil {
		return nil, "", fmt.Errorf("git checkout error: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, "", fmt.Errorf("git resolve HEAD error: %w", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, "", fmt.Errorf("git commit '%s' not found: %w", head.Hash(), err)
	}

	return &Commit{commit}, fmt.Sprintf("%s/%s", t, head.Hash().String()), nil
}

func (c *CheckoutSemVer) ExistsTargetFilesInRecentlyCommits(startHash, endHash, ignore string) (bool, error) {
	return existsTargetFilesInRecentlyCommits(c.repo, startHash, endHash, ignore)
}

func recurseSubmodules(recurse bool) extgogit.SubmoduleRescursivity {
	if recurse {
		return extgogit.DefaultSubmoduleRecursionDepth
	}
	return extgogit.NoRecurseSubmodules
}

func existsTargetFilesInRecentlyCommits(repo *extgogit.Repository, startHash, endHash, ignore string,
) (bool, error) {
	o := extgogit.LogOptions{
		From:  plumbing.NewHash(startHash),
		Order: extgogit.LogOrderCommitterTime,
	}
	commitIter, err := repo.Log(&o)
	if err != nil {
		return false, fmt.Errorf("git log error: %w", err)
	}
	defer commitIter.Close()

	ps := git2.GetIgnorePatterns(bytes.NewBufferString(ignore), nil)
	matcher := gitignore.NewMatcher(ps)
	for {
		c, err := commitIter.Next()
		switch err {
		case nil:
			if c.Hash.String() == endHash {
				return false, nil
			}
		case io.EOF:
			return false, nil
		default:
			return false, fmt.Errorf("git commitIter.Next error: %w", err)
		}

		fs, err := c.Stats()
		if err != nil {
			return false, fmt.Errorf("git commit.Stats error: %w", err)
		}
		for _, f := range fs {
			if match := matcher.Match(strings.Split(f.Name, "/"), false); !match {
				return true, nil
			}
		}
	}
}
