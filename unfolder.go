package tfunfold

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type Unfolder struct {
	Dir       string
	StatePath string
	Out       io.Writer

	files       map[string]*hclwrite.File
	state       map[stateKey][]instanceKey
	moduleAddrs map[string]bool
}

type stateKey struct {
	module string // outermost module address, e.g. "" or `module.foo["a"]`
	mode   string // managed | data
	typ    string
	name   string
}

type instanceKey struct {
	s     string
	n     int
	isInt bool
}

func (k instanceKey) String() string {
	if k.isInt {
		return strconv.Itoa(k.n)
	}
	return k.s
}

func (k instanceKey) tokens() hclwrite.Tokens {
	if k.isInt {
		return hclwrite.TokensForValue(cty.NumberIntVal(int64(k.n)))
	}
	return hclwrite.TokensForValue(cty.StringVal(k.s))
}

type target struct {
	file    string
	block   *hclwrite.Block
	kind    string // resource | module
	typ     string // resource type; "" for module
	name    string
	isCount bool
}

func (t *target) refPrefix() string {
	if t.kind == "module" {
		return "module." + t.name
	}
	return t.typ + "." + t.name
}

type expansion struct {
	target   *target
	keys     []instanceKey
	newNames map[string]string // key.String() -> new identifier
}

func NewUnfolder(dir string) *Unfolder {
	return &Unfolder{
		Dir:       dir,
		StatePath: filepath.Join(dir, "terraform.tfstate"),
		Out:       os.Stdout,
		files:     map[string]*hclwrite.File{},
	}
}

func (u *Unfolder) Unfold(inPlace bool) error {
	if err := u.load(); err != nil {
		return err
	}
	if err := u.loadState(); err != nil {
		return err
	}
	targets := u.collectTargets()
	plan, err := u.plan(targets)
	if err != nil {
		return err
	}
	changed := map[string]bool{}
	u.apply(plan, changed)
	if err := u.rewriteRefs(plan, changed); err != nil {
		return err
	}
	return u.writeOut(inPlace, changed)
}

func (u *Unfolder) load() error {
	pattern := filepath.Join(u.Dir, "*.tf")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob: %w", err)
	}
	sort.Strings(matches)
	var diags hcl.Diagnostics
	for _, path := range matches {
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		f, parseDiags := hclwrite.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
		if parseDiags.HasErrors() {
			diags = append(diags, parseDiags...)
			continue
		}
		u.files[path] = f
	}
	if diags.HasErrors() {
		return diags
	}
	return nil
}

type rawState struct {
	Version   int               `json:"version"`
	Resources []rawStateResource `json:"resources"`
}

type rawStateResource struct {
	Module    string             `json:"module,omitempty"`
	Mode      string             `json:"mode"`
	Type      string             `json:"type"`
	Name      string             `json:"name"`
	Instances []rawStateInstance `json:"instances"`
}

type rawStateInstance struct {
	IndexKey any `json:"index_key,omitempty"`
}

func (u *Unfolder) loadState() error {
	src, err := os.ReadFile(u.StatePath)
	if err != nil {
		return fmt.Errorf("read state %s: %w", u.StatePath, err)
	}
	var raw rawState
	if err := json.Unmarshal(src, &raw); err != nil {
		return fmt.Errorf("parse state %s: %w", u.StatePath, err)
	}
	if raw.Version != 4 {
		return fmt.Errorf("unsupported state version %d (want 4)", raw.Version)
	}
	u.state = map[stateKey][]instanceKey{}
	u.moduleAddrs = map[string]bool{}
	for _, r := range raw.Resources {
		outerModule := outermostModule(r.Module)
		if outerModule != "" {
			u.moduleAddrs[outerModule] = true
		}
		key := stateKey{module: outerModule, mode: r.Mode, typ: r.Type, name: r.Name}
		for _, inst := range r.Instances {
			ik, ok := toInstanceKey(inst.IndexKey)
			if !ok {
				continue
			}
			u.state[key] = append(u.state[key], ik)
		}
	}
	return nil
}

// outermostModule returns the first "module.NAME[KEY]" segment of a state
// module address, or "" if the address has no module prefix.
func outermostModule(addr string) string {
	if addr == "" {
		return ""
	}
	// addr forms: module.foo, module.foo["a"], module.foo.module.bar, module.foo["a"].module.bar
	if !strings.HasPrefix(addr, "module.") {
		return addr
	}
	rest := addr[len("module."):]
	// find end of first segment: either next ".module." or end
	idx := strings.Index(rest, ".module.")
	if idx < 0 {
		return addr
	}
	return "module." + rest[:idx]
}

func toInstanceKey(v any) (instanceKey, bool) {
	switch x := v.(type) {
	case string:
		return instanceKey{s: x}, true
	case float64:
		return instanceKey{n: int(x), isInt: true}, true
	}
	return instanceKey{}, false
}

func (u *Unfolder) collectTargets() []*target {
	var out []*target
	paths := sortedKeys(u.files)
	for _, path := range paths {
		f := u.files[path]
		for _, blk := range f.Body().Blocks() {
			t := blockTarget(path, blk)
			if t == nil {
				continue
			}
			out = append(out, t)
		}
	}
	return out
}

func blockTarget(path string, blk *hclwrite.Block) *target {
	labels := blk.Labels()
	switch blk.Type() {
	case "resource":
		if len(labels) < 2 {
			return nil
		}
		mode, count := hasForEachOrCount(blk.Body())
		if mode == "" {
			return nil
		}
		return &target{
			file:    path,
			block:   blk,
			kind:    "resource",
			typ:     labels[0],
			name:    labels[1],
			isCount: count,
		}
	case "module":
		if len(labels) < 1 {
			return nil
		}
		mode, count := hasForEachOrCount(blk.Body())
		if mode == "" {
			return nil
		}
		return &target{
			file:    path,
			block:   blk,
			kind:    "module",
			name:    labels[0],
			isCount: count,
		}
	}
	return nil
}

func hasForEachOrCount(body *hclwrite.Body) (mode string, isCount bool) {
	if body.GetAttribute("for_each") != nil {
		return "for_each", false
	}
	if body.GetAttribute("count") != nil {
		return "count", true
	}
	return "", false
}

func (u *Unfolder) plan(targets []*target) ([]*expansion, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	existingNames := u.collectExistingNames()
	var out []*expansion
	for _, t := range targets {
		keys, err := u.lookupKeys(t)
		if err != nil {
			return nil, err
		}
		newNames, err := assignNewNames(t, keys, existingNames)
		if err != nil {
			return nil, err
		}
		out = append(out, &expansion{target: t, keys: keys, newNames: newNames})
	}
	return out, nil
}

// collectExistingNames returns the set of "resource:<type>.<name>" and
// "module:<name>" identifiers already declared in the loaded files. The
// expansion's new names are validated against this set to prevent collisions.
func (u *Unfolder) collectExistingNames() map[string]bool {
	out := map[string]bool{}
	for _, f := range u.files {
		for _, blk := range f.Body().Blocks() {
			labels := blk.Labels()
			switch blk.Type() {
			case "resource":
				if len(labels) >= 2 {
					out["resource:"+labels[0]+"."+labels[1]] = true
				}
			case "module":
				if len(labels) >= 1 {
					out["module:"+labels[0]] = true
				}
			}
		}
	}
	return out
}

func (u *Unfolder) lookupKeys(t *target) ([]instanceKey, error) {
	if t.kind == "resource" {
		key := stateKey{module: "", mode: "managed", typ: t.typ, name: t.name}
		keys := u.state[key]
		if len(keys) == 0 {
			return nil, fmt.Errorf("%s: no state instances for resource %s.%s", t.file, t.typ, t.name)
		}
		return sortKeys(dedupeKeys(keys)), nil
	}
	// module: collect distinct keys from outermost-module addresses matching module.<name>[...]
	prefix := "module." + t.name + "["
	seen := map[string]instanceKey{}
	for addr := range u.moduleAddrs {
		if !strings.HasPrefix(addr, prefix) {
			continue
		}
		inside := addr[len(prefix):]
		end := strings.LastIndex(inside, "]")
		if end < 0 {
			continue
		}
		ik, ok := parseSubscript(inside[:end])
		if !ok {
			continue
		}
		seen[ik.String()] = ik
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("%s: no state instances for module %s", t.file, t.name)
	}
	out := make([]instanceKey, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	return sortKeys(out), nil
}

// parseSubscript parses the contents between [ and ] of a state address
// subscript: `"a"` -> string key, `0` -> int key.
func parseSubscript(s string) (instanceKey, bool) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		unq, err := strconv.Unquote(s)
		if err != nil {
			return instanceKey{}, false
		}
		return instanceKey{s: unq}, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return instanceKey{}, false
	}
	return instanceKey{n: n, isInt: true}, true
}

func assignNewNames(t *target, keys []instanceKey, existing map[string]bool) (map[string]string, error) {
	out := map[string]string{}
	used := map[string]bool{}
	for _, k := range keys {
		name := t.name + "_" + sanitizeKey(k.String())
		if used[name] {
			return nil, fmt.Errorf("%s: cannot assign unique name for key %q (collides with another expanded key)", t.file, k.String())
		}
		var existingKind string
		if t.kind == "module" {
			existingKind = "module:" + name
		} else {
			existingKind = "resource:" + t.typ + "." + name
		}
		if existing[existingKind] {
			return nil, fmt.Errorf("%s: expanded name %q collides with existing %s", t.file, name, t.kind)
		}
		used[name] = true
		out[k.String()] = name
	}
	return out, nil
}

var nonIdentRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func sanitizeKey(s string) string {
	if s == "" {
		return "empty"
	}
	return nonIdentRe.ReplaceAllString(s, "_")
}

func (u *Unfolder) apply(plan []*expansion, changed map[string]bool) {
	for _, exp := range plan {
		body := u.files[exp.target.file].Body()
		body.RemoveBlock(exp.target.block)
		for _, k := range exp.keys {
			newName := exp.newNames[k.String()]
			body.AppendNewline()
			appendMoved(body, exp.target, k, newName)
			appendCloned(body, exp.target, k, newName)
		}
		changed[exp.target.file] = true
	}
}

func appendMoved(body *hclwrite.Body, t *target, k instanceKey, newName string) {
	blk := body.AppendNewBlock("moved", nil)
	blk.Body().SetAttributeRaw("from", addressTokens(t, &k))
	blk.Body().SetAttributeRaw("to", addressTokens(&target{kind: t.kind, typ: t.typ, name: newName}, nil))
}

func addressTokens(t *target, k *instanceKey) hclwrite.Tokens {
	var trav hcl.Traversal
	if t.kind == "module" {
		trav = hcl.Traversal{
			hcl.TraverseRoot{Name: "module"},
			hcl.TraverseAttr{Name: t.name},
		}
	} else {
		trav = hcl.Traversal{
			hcl.TraverseRoot{Name: t.typ},
			hcl.TraverseAttr{Name: t.name},
		}
	}
	if k != nil {
		if k.isInt {
			trav = append(trav, hcl.TraverseIndex{Key: cty.NumberIntVal(int64(k.n))})
		} else {
			trav = append(trav, hcl.TraverseIndex{Key: cty.StringVal(k.s)})
		}
	}
	return hclwrite.TokensForTraversal(trav)
}

func appendCloned(body *hclwrite.Body, t *target, k instanceKey, newName string) {
	var labels []string
	if t.kind == "module" {
		labels = []string{newName}
	} else {
		labels = []string{t.typ, newName}
	}
	blk := body.AppendNewBlock(t.kind, labels)
	copyBodyExpanded(t.block.Body(), blk.Body(), k, t.isCount)
}

func copyBodyExpanded(src, dst *hclwrite.Body, k instanceKey, isCount bool) {
	for _, name := range orderedAttrNames(src) {
		if name == "for_each" || name == "count" {
			continue
		}
		attr := src.GetAttribute(name)
		tokens := substituteEachCount(attr.Expr().BuildTokens(nil), k, isCount)
		dst.SetAttributeRaw(name, tokens)
	}
	for _, blk := range src.Blocks() {
		nested := dst.AppendNewBlock(blk.Type(), blk.Labels())
		copyBodyExpanded(blk.Body(), nested.Body(), k, isCount)
	}
}

// orderedAttrNames returns attribute names in source order by walking the
// body's token stream. hclwrite's Attributes() returns a map, so it cannot
// preserve order on its own.
func orderedAttrNames(b *hclwrite.Body) []string {
	attrs := b.Attributes()
	tokens := b.BuildTokens(nil)
	var out []string
	depth := 0
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack:
			depth++
		case hclsyntax.TokenCBrace, hclsyntax.TokenCBrack:
			depth--
		case hclsyntax.TokenIdent:
			if depth != 0 {
				continue
			}
			if i+1 >= len(tokens) || tokens[i+1].Type != hclsyntax.TokenEqual {
				continue
			}
			name := string(t.Bytes)
			if _, ok := attrs[name]; ok {
				out = append(out, name)
			}
		}
	}
	return out
}

func substituteEachCount(tokens hclwrite.Tokens, k instanceKey, isCount bool) hclwrite.Tokens {
	var out hclwrite.Tokens
	for i := 0; i < len(tokens); i++ {
		matched := false
		if isCount && matchCountIndex(tokens, i) {
			matched = true
		} else if !isCount && matchEachRef(tokens, i) {
			matched = true
		}
		if !matched {
			out = append(out, tokens[i])
			continue
		}
		// If this 3-token match is the entire content of a ${...} template
		// interpolation, splice the value into the surrounding template as a
		// QuotedLit so the result is "prefix-a" rather than "prefix-${"a"}".
		if i > 0 && i+3 < len(tokens) &&
			tokens[i-1].Type == hclsyntax.TokenTemplateInterp &&
			tokens[i+3].Type == hclsyntax.TokenTemplateSeqEnd {
			out = out[:len(out)-1]
			out = append(out, &hclwrite.Token{
				Type:  hclsyntax.TokenQuotedLit,
				Bytes: []byte(escapeQuotedLit(k.String())),
			})
			i += 3
			continue
		}
		out = append(out, k.tokens()...)
		i += 2
	}
	return out
}

func escapeQuotedLit(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `$$`)
	s = strings.ReplaceAll(s, `%`, `%%`)
	return s
}

func matchEachRef(tokens hclwrite.Tokens, i int) bool {
	if !matchThreeTok(tokens, i, "each") {
		return false
	}
	leaf := string(tokens[i+2].Bytes)
	return leaf == "key" || leaf == "value"
}

func matchCountIndex(tokens hclwrite.Tokens, i int) bool {
	if !matchThreeTok(tokens, i, "count") {
		return false
	}
	return string(tokens[i+2].Bytes) == "index"
}

func matchThreeTok(tokens hclwrite.Tokens, i int, head string) bool {
	if i+2 >= len(tokens) {
		return false
	}
	t0, t1, t2 := tokens[i], tokens[i+1], tokens[i+2]
	if t0.Type != hclsyntax.TokenIdent || string(t0.Bytes) != head {
		return false
	}
	if t1.Type != hclsyntax.TokenDot {
		return false
	}
	if t2.Type != hclsyntax.TokenIdent {
		return false
	}
	if i > 0 && tokens[i-1].Type == hclsyntax.TokenDot {
		return false
	}
	return true
}

func (u *Unfolder) rewriteRefs(plan []*expansion, changed map[string]bool) error {
	if len(plan) == 0 {
		return nil
	}
	idx := buildRefIndex(plan)
	paths := sortedKeys(u.files)
	for _, path := range paths {
		f := u.files[path]
		fileChanged, err := rewriteBody(f.Body(), idx, path, true)
		if err != nil {
			return err
		}
		if fileChanged {
			changed[path] = true
		}
	}
	return nil
}

type refIndex struct {
	// key: "resource:<type>.<name>" or "module:<name>" -> expansion
	byTarget map[string]*expansion
}

func buildRefIndex(plan []*expansion) *refIndex {
	idx := &refIndex{byTarget: map[string]*expansion{}}
	for _, exp := range plan {
		t := exp.target
		if t.kind == "module" {
			idx.byTarget["module:"+t.name] = exp
		} else {
			idx.byTarget["resource:"+t.typ+"."+t.name] = exp
		}
	}
	return idx
}

// rewriteBody walks a body recursively and rewrites every attribute whose
// expression references an expanded target. Returns whether anything was
// written, and any unresolvable-reference error. skipMoved suppresses rewrites
// inside `moved` blocks (their `from` must stay as the old address).
func rewriteBody(body *hclwrite.Body, idx *refIndex, path string, skipMoved bool) (bool, error) {
	changed := false
	for _, name := range orderedAttrNames(body) {
		attr := body.GetAttribute(name)
		tokens := attr.Expr().BuildTokens(nil)
		rewritten, hit, err := rewriteTokens(tokens, idx, path)
		if err != nil {
			return changed, err
		}
		if hit {
			body.SetAttributeRaw(name, rewritten)
			changed = true
		}
	}
	for _, blk := range body.Blocks() {
		if skipMoved && blk.Type() == "moved" {
			continue
		}
		c, err := rewriteBody(blk.Body(), idx, path, true)
		if err != nil {
			return changed, err
		}
		if c {
			changed = true
		}
	}
	return changed, nil
}

// rewriteTokens scans a token stream for references to expanded targets and
// returns a rewritten copy plus a flag for whether anything matched. It errors
// when a reference cannot be statically rewritten (dynamic subscript or
// whole-collection access).
func rewriteTokens(tokens hclwrite.Tokens, idx *refIndex, path string) (hclwrite.Tokens, bool, error) {
	var out hclwrite.Tokens
	hit := false
	for i := 0; i < len(tokens); i++ {
		matched, exp, headLen := matchTargetRef(tokens, i, idx)
		if !matched {
			out = append(out, tokens[i])
			continue
		}
		next := i + headLen
		if isLiteralSubscript(tokens, next) {
			ik, subLen := readLiteralKey(tokens, next)
			newName, ok := exp.newNames[ik.String()]
			if !ok {
				return nil, false, fmt.Errorf("%s: reference %s%s has no matching state instance", path, exp.target.refPrefix(), subscriptStr(ik))
			}
			out = append(out, renamedRefTokens(exp.target, newName)...)
			i = next + subLen - 1
			hit = true
			continue
		}
		if next < len(tokens) && tokens[next].Type == hclsyntax.TokenOBrack {
			return nil, false, fmt.Errorf("%s: %s has dynamic subscript, cannot statically rewrite", path, exp.target.refPrefix())
		}
		return nil, false, fmt.Errorf("%s: %s referenced without subscript (whole-collection), cannot statically rewrite", path, exp.target.refPrefix())
	}
	return out, hit, nil
}

// matchTargetRef checks whether tokens at position i begin a reference to an
// expanded target. Returns matched, the expansion, and the number of head
// tokens (e.g., `aws_x.y` is 3 tokens; `module.foo` is 3 tokens).
func matchTargetRef(tokens hclwrite.Tokens, i int, idx *refIndex) (bool, *expansion, int) {
	if i+2 >= len(tokens) {
		return false, nil, 0
	}
	t0, t1, t2 := tokens[i], tokens[i+1], tokens[i+2]
	if t0.Type != hclsyntax.TokenIdent {
		return false, nil, 0
	}
	if t1.Type != hclsyntax.TokenDot {
		return false, nil, 0
	}
	if t2.Type != hclsyntax.TokenIdent {
		return false, nil, 0
	}
	if i > 0 && tokens[i-1].Type == hclsyntax.TokenDot {
		return false, nil, 0
	}
	head := string(t0.Bytes)
	name := string(t2.Bytes)
	if head == "module" {
		if exp, ok := idx.byTarget["module:"+name]; ok {
			return true, exp, 3
		}
		return false, nil, 0
	}
	if exp, ok := idx.byTarget["resource:"+head+"."+name]; ok {
		return true, exp, 3
	}
	return false, nil, 0
}

func isLiteralSubscript(tokens hclwrite.Tokens, i int) bool {
	if i+2 >= len(tokens) {
		return false
	}
	if tokens[i].Type != hclsyntax.TokenOBrack {
		return false
	}
	// [ "..." ] (3 tokens via OQuote..CQuote actually 5) OR [ NUMBER ] (3 tokens)
	// Number form: OBrack, NumberLit, CBrack
	if tokens[i+1].Type == hclsyntax.TokenNumberLit && tokens[i+2].Type == hclsyntax.TokenCBrack {
		return true
	}
	// String form: OBrack, OQuote, QuotedLit, CQuote, CBrack
	if i+4 < len(tokens) &&
		tokens[i+1].Type == hclsyntax.TokenOQuote &&
		tokens[i+2].Type == hclsyntax.TokenQuotedLit &&
		tokens[i+3].Type == hclsyntax.TokenCQuote &&
		tokens[i+4].Type == hclsyntax.TokenCBrack {
		return true
	}
	return false
}

// readLiteralKey extracts the literal key from a [...] subscript expression
// starting at tokens[i]. The second return value is the number of tokens the
// subscript occupies (3 for [N], 5 for ["s"]). Caller must have validated the
// shape with isLiteralSubscript first.
func readLiteralKey(tokens hclwrite.Tokens, i int) (instanceKey, int) {
	if tokens[i+1].Type == hclsyntax.TokenNumberLit {
		n, _ := strconv.Atoi(string(tokens[i+1].Bytes))
		return instanceKey{n: n, isInt: true}, 3
	}
	return instanceKey{s: string(tokens[i+2].Bytes)}, 5
}

func renamedRefTokens(t *target, newName string) hclwrite.Tokens {
	if t.kind == "module" {
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "module"},
			hcl.TraverseAttr{Name: newName},
		})
	}
	return hclwrite.TokensForTraversal(hcl.Traversal{
		hcl.TraverseRoot{Name: t.typ},
		hcl.TraverseAttr{Name: newName},
	})
}

func subscriptStr(k instanceKey) string {
	if k.isInt {
		return "[" + strconv.Itoa(k.n) + "]"
	}
	return "[" + strconv.Quote(k.s) + "]"
}

func (u *Unfolder) writeOut(inPlace bool, changedFiles map[string]bool) error {
	paths := sortedKeys(u.files)
	for _, path := range paths {
		if !changedFiles[path] {
			continue
		}
		f := u.files[path]
		body := f.Bytes()
		if !inPlace {
			if _, err := fmt.Fprintf(u.Out, "### %s ###\n%s", path, body); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupeKeys(in []instanceKey) []instanceKey {
	seen := map[string]bool{}
	var out []instanceKey
	for _, k := range in {
		s := k.String() + "/" + strconv.FormatBool(k.isInt)
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, k)
	}
	return out
}

func sortKeys(in []instanceKey) []instanceKey {
	out := make([]instanceKey, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool {
		if out[i].isInt != out[j].isInt {
			return out[i].isInt
		}
		if out[i].isInt {
			return out[i].n < out[j].n
		}
		return out[i].s < out[j].s
	})
	return out
}
