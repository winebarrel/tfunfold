package tfunfold_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/winebarrel/tfunfold"
)

func TestUnfold_Golden(t *testing.T) {
	cases := []string{
		"basic-resource",
		"count",
		"module-foreach",
		"module-ref",
		"nested-block",
		"cross-file-ref",
		"odd-blocks",
		"preceded-by-dot",
		"no-changes",
		"sanitize-key",
		"malformed-state",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := copyInputToTemp(t, filepath.Join("testdata", name, "input"))
			u := tfunfold.NewUnfolder(tmp)
			require.NoError(t, u.Unfold(true))
			compareDir(t, tmp, filepath.Join("testdata", name, "expected"))
		})
	}
}

func TestUnfold_Errors(t *testing.T) {
	cases := []string{
		"parse-error",
		"bad-state-version",
		"bad-state-json",
		"missing-state",
		"missing-state-module",
		"collision",
		"unresolvable-dynamic",
		"unresolvable-whole",
		"unknown-key",
		"duplicate-sanitized",
		"module-dynamic",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := copyInputToTemp(t, filepath.Join("testdata", name, "input"))
			u := tfunfold.NewUnfolder(tmp)
			require.Error(t, u.Unfold(true))
		})
	}
}

func TestUnfold_StdoutMode(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic-resource/input")
	var buf bytes.Buffer
	u := tfunfold.NewUnfolder(tmp)
	u.Out = &buf
	require.NoError(t, u.Unfold(false))
	assert.Contains(t, buf.String(), `null_resource.x_a`)
	assert.Contains(t, buf.String(), `moved {`)
	got, err := os.ReadFile(filepath.Join(tmp, "main.tf"))
	require.NoError(t, err)
	want, err := os.ReadFile("testdata/basic-resource/input/main.tf")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got), "file must not be modified in stdout mode")
}

func TestUnfold_StdoutNoChange(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/no-changes/input")
	var buf bytes.Buffer
	u := tfunfold.NewUnfolder(tmp)
	u.Out = &buf
	require.NoError(t, u.Unfold(false))
	assert.Empty(t, buf.String())
}

func TestUnfold_StdoutWriteError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic-resource/input")
	u := tfunfold.NewUnfolder(tmp)
	u.Out = failingWriter{}
	require.Error(t, u.Unfold(false))
}

func TestUnfold_GlobError(t *testing.T) {
	u := tfunfold.NewUnfolder("[invalid")
	err := u.Unfold(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "glob")
}

func TestUnfold_LoadReadError(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tmp, "trap.tf"), 0o755))
	u := tfunfold.NewUnfolder(tmp)
	require.Error(t, u.Unfold(true))
}

func TestUnfold_StateReadError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/no-changes/input")
	require.NoError(t, os.Remove(filepath.Join(tmp, "terraform.tfstate")))
	u := tfunfold.NewUnfolder(tmp)
	require.Error(t, u.Unfold(true))
}

func TestUnfold_InPlaceWriteError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic-resource/input")
	require.NoError(t, os.Chmod(filepath.Join(tmp, "main.tf"), 0o444))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(tmp, "main.tf"), 0o644) })
	u := tfunfold.NewUnfolder(tmp)
	require.Error(t, u.Unfold(true))
}

// ----------------- unit tests for internal helpers -----------------

func TestOutermostModule(t *testing.T) {
	assert.Equal(t, "", tfunfold.OutermostModule(""))
	assert.Equal(t, "weird", tfunfold.OutermostModule("weird"))
	assert.Equal(t, `module.foo["a"]`, tfunfold.OutermostModule(`module.foo["a"]`))
	assert.Equal(t, "module.foo", tfunfold.OutermostModule("module.foo"))
	assert.Equal(t, `module.foo["a"]`, tfunfold.OutermostModule(`module.foo["a"].module.bar`))
}

func TestToInstanceKey(t *testing.T) {
	k, ok := tfunfold.ToInstanceKey("a")
	require.True(t, ok)
	assert.Equal(t, "a", k.StringForTest())
	assert.False(t, k.IsIntForTest())

	k, ok = tfunfold.ToInstanceKey(float64(3))
	require.True(t, ok)
	assert.True(t, k.IsIntForTest())
	assert.Equal(t, 3, k.IntForTest())

	_, ok = tfunfold.ToInstanceKey(nil)
	assert.False(t, ok)

	_, ok = tfunfold.ToInstanceKey([]any{1, 2})
	assert.False(t, ok)
}

func TestParseSubscript(t *testing.T) {
	k, ok := tfunfold.ParseSubscript(`"a"`)
	require.True(t, ok)
	assert.Equal(t, "a", k.StringForTest())

	k, ok = tfunfold.ParseSubscript(`0`)
	require.True(t, ok)
	assert.True(t, k.IsIntForTest())

	_, ok = tfunfold.ParseSubscript(`"unterminated`)
	assert.False(t, ok)

	_, ok = tfunfold.ParseSubscript(`abc`)
	assert.False(t, ok)

	_, ok = tfunfold.ParseSubscript(`"\xZZ"`)
	assert.False(t, ok)
}

func TestSanitizeKey(t *testing.T) {
	assert.Equal(t, "empty", tfunfold.SanitizeKey(""))
	assert.Equal(t, "abc", tfunfold.SanitizeKey("abc"))
	assert.Equal(t, "a-b-c", tfunfold.SanitizeKey("a-b-c"))
	assert.Equal(t, "a_b_c", tfunfold.SanitizeKey("a.b.c"))
	assert.Equal(t, "0", tfunfold.SanitizeKey("0"))
}

func TestEscapeQuotedLit(t *testing.T) {
	assert.Equal(t, "abc", tfunfold.EscapeQuotedLit("abc"))
	assert.Equal(t, `\\`, tfunfold.EscapeQuotedLit(`\`))
	assert.Equal(t, `\"`, tfunfold.EscapeQuotedLit(`"`))
	assert.Equal(t, "$$", tfunfold.EscapeQuotedLit("$"))
	assert.Equal(t, "%%", tfunfold.EscapeQuotedLit("%"))
}

func TestSubscriptStr(t *testing.T) {
	assert.Equal(t, `["a"]`, tfunfold.SubscriptStr(tfunfold.NewInstanceKeyString("a")))
	assert.Equal(t, `[3]`, tfunfold.SubscriptStr(tfunfold.NewInstanceKeyInt(3)))
}

func TestMatchEachRef_Branches(t *testing.T) {
	mk := func(s string) []byte { return []byte(s) }
	ident := func(s string) *hclwrite.Token {
		return &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: mk(s)}
	}
	dot := &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: mk(".")}

	// Too short.
	assert.False(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("each")}, 0))

	// Not "each".
	assert.False(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("foo"), dot, ident("key")}, 0))

	// Middle not dot.
	assert.False(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("each"), ident("key"), ident("key")}, 0))

	// Third not ident.
	assert.False(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("each"), dot, dot}, 0))

	// Leaf not key/value.
	assert.False(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("each"), dot, ident("foo")}, 0))

	// Preceded by dot.
	assert.False(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("x"), dot, ident("each"), dot, ident("key")}, 2))

	// Match key.
	assert.True(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("each"), dot, ident("key")}, 0))

	// Match value.
	assert.True(t, tfunfold.MatchEachRef(hclwrite.Tokens{ident("each"), dot, ident("value")}, 0))
}

func TestMatchCountIndex_Branches(t *testing.T) {
	mk := func(s string) []byte { return []byte(s) }
	ident := func(s string) *hclwrite.Token {
		return &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: mk(s)}
	}
	dot := &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: mk(".")}

	assert.True(t, tfunfold.MatchCountIndex(hclwrite.Tokens{ident("count"), dot, ident("index")}, 0))
	assert.False(t, tfunfold.MatchCountIndex(hclwrite.Tokens{ident("count"), dot, ident("nope")}, 0))
}

func TestIsLiteralSubscript(t *testing.T) {
	mk := func(s string) []byte { return []byte(s) }
	tok := func(ty hclsyntax.TokenType, s string) *hclwrite.Token {
		return &hclwrite.Token{Type: ty, Bytes: mk(s)}
	}

	// Too short.
	assert.False(t, tfunfold.IsLiteralSubscript(hclwrite.Tokens{tok(hclsyntax.TokenOBrack, "[")}, 0))

	// Not opening bracket.
	assert.False(t, tfunfold.IsLiteralSubscript(hclwrite.Tokens{
		tok(hclsyntax.TokenIdent, "x"),
		tok(hclsyntax.TokenNumberLit, "0"),
		tok(hclsyntax.TokenCBrack, "]"),
	}, 0))

	// Number form.
	assert.True(t, tfunfold.IsLiteralSubscript(hclwrite.Tokens{
		tok(hclsyntax.TokenOBrack, "["),
		tok(hclsyntax.TokenNumberLit, "5"),
		tok(hclsyntax.TokenCBrack, "]"),
	}, 0))

	// String form.
	assert.True(t, tfunfold.IsLiteralSubscript(hclwrite.Tokens{
		tok(hclsyntax.TokenOBrack, "["),
		tok(hclsyntax.TokenOQuote, `"`),
		tok(hclsyntax.TokenQuotedLit, "a"),
		tok(hclsyntax.TokenCQuote, `"`),
		tok(hclsyntax.TokenCBrack, "]"),
	}, 0))

	// Neither form.
	assert.False(t, tfunfold.IsLiteralSubscript(hclwrite.Tokens{
		tok(hclsyntax.TokenOBrack, "["),
		tok(hclsyntax.TokenIdent, "x"),
		tok(hclsyntax.TokenCBrack, "]"),
	}, 0))
}

func TestReadLiteralKey(t *testing.T) {
	mk := func(s string) []byte { return []byte(s) }
	tok := func(ty hclsyntax.TokenType, s string) *hclwrite.Token {
		return &hclwrite.Token{Type: ty, Bytes: mk(s)}
	}

	k, n := tfunfold.ReadLiteralKey(hclwrite.Tokens{
		tok(hclsyntax.TokenOBrack, "["),
		tok(hclsyntax.TokenNumberLit, "5"),
		tok(hclsyntax.TokenCBrack, "]"),
	}, 0)
	assert.True(t, k.IsIntForTest())
	assert.Equal(t, 5, k.IntForTest())
	assert.Equal(t, 3, n)

	k, n = tfunfold.ReadLiteralKey(hclwrite.Tokens{
		tok(hclsyntax.TokenOBrack, "["),
		tok(hclsyntax.TokenOQuote, `"`),
		tok(hclsyntax.TokenQuotedLit, "abc"),
		tok(hclsyntax.TokenCQuote, `"`),
		tok(hclsyntax.TokenCBrack, "]"),
	}, 0)
	assert.False(t, k.IsIntForTest())
	assert.Equal(t, "abc", k.StringForTest())
	assert.Equal(t, 5, n)
}

func TestSubstituteEachCount_InsideTemplate(t *testing.T) {
	src := []byte(`x = "p-${each.key}-q"`)
	f, diags := hclwrite.ParseConfig(src, "t.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	tokens := f.Body().GetAttribute("x").Expr().BuildTokens(nil)
	out := tfunfold.SubstituteEachCount(tokens, tfunfold.NewInstanceKeyString("a"), false)
	rendered := string(hclwrite.Format(out.Bytes()))
	assert.Contains(t, rendered, `"p-a-q"`)
}

func TestSubstituteEachCount_BareReference(t *testing.T) {
	src := []byte(`x = each.key`)
	f, diags := hclwrite.ParseConfig(src, "t.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	tokens := f.Body().GetAttribute("x").Expr().BuildTokens(nil)
	out := tfunfold.SubstituteEachCount(tokens, tfunfold.NewInstanceKeyString("a"), false)
	rendered := string(hclwrite.Format(out.Bytes()))
	assert.Contains(t, rendered, `"a"`)
}

func TestSubstituteEachCount_CountIndex(t *testing.T) {
	src := []byte(`x = count.index`)
	f, diags := hclwrite.ParseConfig(src, "t.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	tokens := f.Body().GetAttribute("x").Expr().BuildTokens(nil)
	out := tfunfold.SubstituteEachCount(tokens, tfunfold.NewInstanceKeyInt(7), true)
	rendered := string(hclwrite.Format(out.Bytes()))
	assert.Contains(t, rendered, "7")
}

func TestDedupeKeys(t *testing.T) {
	in := []tfunfold.InstanceKey{
		tfunfold.NewInstanceKeyString("a"),
		tfunfold.NewInstanceKeyString("a"),
		tfunfold.NewInstanceKeyString("b"),
		tfunfold.NewInstanceKeyInt(0),
		tfunfold.NewInstanceKeyInt(0),
	}
	out := tfunfold.DedupeKeys(in)
	assert.Len(t, out, 3)
}

func TestSortKeys(t *testing.T) {
	in := []tfunfold.InstanceKey{
		tfunfold.NewInstanceKeyString("b"),
		tfunfold.NewInstanceKeyInt(1),
		tfunfold.NewInstanceKeyString("a"),
		tfunfold.NewInstanceKeyInt(0),
	}
	out := tfunfold.SortKeys(in)
	// ints first (we treat isInt=true as "less"), then strings sorted.
	assert.True(t, out[0].IsIntForTest())
	assert.True(t, out[1].IsIntForTest())
	assert.Equal(t, 0, out[0].IntForTest())
	assert.Equal(t, 1, out[1].IntForTest())
	assert.Equal(t, "a", out[2].StringForTest())
	assert.Equal(t, "b", out[3].StringForTest())
}

func TestOrderedAttrNames(t *testing.T) {
	src := []byte(`
b = 1
a = 2
nested {
  inner = 3
}
c = 4
`)
	f, diags := hclwrite.ParseConfig(src, "t.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	names := tfunfold.OrderedAttrNames(f.Body())
	assert.Equal(t, []string{"b", "a", "c"}, names)
}

// ----------------- helpers -----------------

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) { return 0, errors.New("boom") }

func copyInputToTemp(t *testing.T, srcDir string) string {
	t.Helper()
	tmp := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, ent.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(tmp, ent.Name()), data, 0o644))
	}
	return tmp
}

func compareDir(t *testing.T, gotDir, wantDir string) {
	t.Helper()
	entries, err := os.ReadDir(wantDir)
	require.NoError(t, err)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		got, err := os.ReadFile(filepath.Join(gotDir, ent.Name()))
		require.NoError(t, err)
		want, err := os.ReadFile(filepath.Join(wantDir, ent.Name()))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "file %s", ent.Name())
	}
}
