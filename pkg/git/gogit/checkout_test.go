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
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/fluxcd/source-controller/pkg/git"
)

func TestCheckoutTagSemVer_Checkout(t *testing.T) {
	auth := &git.Auth{}
	tag := CheckoutTag{
		tag: "v1.7.0",
	}
	tmpDir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpDir)

	cTag, _, err := tag.Checkout(context.TODO(), tmpDir, "https://github.com/projectcontour/contour", auth)
	if err != nil {
		t.Error(err)
	}

	semVer := CheckoutSemVer{
		semVer: ">=1.0.0 <=1.7.0",
	}
	tmpDir2, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpDir2)

	cSemVer, _, err := semVer.Checkout(context.TODO(), tmpDir2, "https://github.com/projectcontour/contour", auth)
	if err != nil {
		t.Error(err)
	}

	if cTag.Hash() != cSemVer.Hash() {
		t.Errorf("expected semver hash %s, got %s", cTag.Hash(), cSemVer.Hash())
	}
}

func TestCheckoutTag_ExistsTargetFilesInRecentlyCommits(t *testing.T) {
	auth := &git.Auth{}
	commit := CheckoutCommit{
		branch: "main",
		commit: "c55d96a6419238a6b3574cd130caf29b9b7d0fff",
	}
	tmpDir, _ := ioutil.TempDir("", "test")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	if _, _, err := commit.Checkout(context.TODO(), tmpDir, "https://github.com/projectcontour/contour", auth); err != nil {
		t.Fatal(err)
	}

	ts := []struct {
		fromHash string
		toHash   string
		ignore   string
		want     bool
	}{
		{
			fromHash: "c55d96a6419238a6b3574cd130caf29b9b7d0fff",
			toHash:   "b4f543f3aa350d692079b00ee28e6f5f07efc68f",
			ignore: `
# comment
/*
!/base
`,
			want: false,
		},
		{
			fromHash: "c55d96a6419238a6b3574cd130caf29b9b7d0fff",
			toHash:   "73ee93e004bb70e4464221ed792117564c89ae54",
			ignore: `
site/
internal/dag/
`,
			want: false,
		},
		{
			fromHash: "c55d96a6419238a6b3574cd130caf29b9b7d0fff",
			toHash:   "73ee93e004bb70e4464221ed792117564c89ae54",
			ignore: `
# comment
/*
!/site
`,
			want: true,
		},
	}
	for i, tt := range ts {
		exists, err := commit.ExistsTargetFilesInRecentlyCommits(tt.fromHash, tt.toHash, tt.ignore)
		if err != nil {
			t.Error(err)
		}
		if exists != tt.want {
			t.Errorf("index: %d expected exists %t, got %t", i, tt.want, exists)
		}
	}
}
