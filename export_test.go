package tfunfold

var (
	OutermostModule    = outermostModule
	ToInstanceKey      = toInstanceKey
	ParseSubscript     = parseSubscript
	SanitizeKey        = sanitizeKey
	EscapeQuotedLit    = escapeQuotedLit
	SubscriptStr       = subscriptStr
	OrderedAttrNames   = orderedAttrNames
	SubstituteEachCount = substituteEachCount
	MatchEachRef       = matchEachRef
	MatchCountIndex    = matchCountIndex
	IsLiteralSubscript = isLiteralSubscript
	ReadLiteralKey     = readLiteralKey
	DedupeKeys         = dedupeKeys
	SortKeys           = sortKeys
)

type InstanceKey = instanceKey

func NewInstanceKeyString(s string) InstanceKey       { return instanceKey{s: s} }
func NewInstanceKeyInt(n int) InstanceKey             { return instanceKey{n: n, isInt: true} }
func (k InstanceKey) StringForTest() string           { return k.String() }
func (k InstanceKey) IsIntForTest() bool              { return k.isInt }
func (k InstanceKey) IntForTest() int                 { return k.n }
