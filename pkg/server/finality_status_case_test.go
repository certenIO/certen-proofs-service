package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// RB12-OPS-C item 1 (F11). encoding/json matches a struct member's name case-insensitively, so a status document that spells a member in
// another case (PINSSHA256 for pinsSha256) was read as the member. The contract with the validator writer is exact: the writer
// (validator pkg/finality BuilderStatus.Document) spells every member exactly, so a case variant is a document no writer produced and
// is refused by name, on every strictly read structure: the document, the named state, and the state's fields.

const memberCaseReason = "member_name_case"

// the members of certen-finality-status/1 as the validator's statusDoc spells them (validator pkg/finality/status.go)
var contractStatusMembers = []string{"format", "class", "levelReached", "nextLevel", "eta", "seq", "state", "pinsSha256"}

func TestTheStatusDocumentMembersAreExactlyTheValidatorsContract(t *testing.T) {
	var got []string
	st := reflect.TypeOf(statusDocument{})
	for i := 0; i < st.NumField(); i++ {
		name, _, _ := strings.Cut(st.Field(i).Tag.Get("json"), ",")
		got = append(got, name)
	}
	want := append([]string(nil), contractStatusMembers...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the reader's members %v differ from the writer's %v", got, want)
	}
}

func variantsOf(name string) []string {
	return []string{strings.ToUpper(name), strings.ToLower(name), strings.ToUpper(name[:1]) + name[1:], strings.ToLower(name[:1]) + strings.ToUpper(name[1:])}
}

func respell(doc, member, variant string) string {
	return strings.Replace(doc, `"`+member+`":`, `"`+variant+`":`, 1)
}

func summarizeStatus(t *testing.T, doc string) (*FinalitySummary, error) {
	t.Helper()
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	return SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc)})
}

func TestACaseVariantOfAnyStatusMemberIsRefusedByNameAndNeverRead(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	docs := map[string]string{
		"pending with pins and eta": withPins(pendingDoc, pinsA),
		"verified with seq":         withPins(verifiedDoc, pinsA),
	}
	for docName, doc := range docs {
		for _, member := range contractStatusMembers {
			if !strings.Contains(doc, `"`+member+`":`) {
				continue
			}
			for _, variant := range variantsOf(member) {
				if variant == member {
					continue
				}
				bad := respell(doc, member, variant)
				_, err := summarizeStatus(t, bad)
				_, reason := malformedReason(t, err)
				if reason != memberCaseReason {
					t.Errorf("%s: %s spelled %s: reason %q", docName, member, variant, reason)
					continue
				}
				var named *FinalityComposeError
				if !errors.As(err, &named) || named.Fields["member"] != member {
					t.Errorf("%s: %s spelled %s: the refusal does not name the member: %v", docName, member, variant, err)
				}
				if strings.Contains(err.Error(), variant) {
					t.Errorf("%s: the refusal echoes the stored spelling %q: %v", docName, variant, err)
				}
				// through the endpoint: the whole answer is the named 500, the variant is not echoed, and nothing is served as the member
				rr := getFinality(NewFinalityHandlers(&fakeSummaryStore{exists: true, statuses: []database.FinalityStatus{statusRow(proof, 84532, 3, bad)}}, nil), proof.String())
				if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "FINALITY_SUMMARY_UNUSABLE") || !strings.Contains(rr.Body.String(), memberCaseReason) {
					t.Errorf("%s: %s spelled %s: %d %s", docName, member, variant, rr.Code, rr.Body)
				}
			}
		}
	}
}

// a document that carries the exact member AND a spelling of it in another case is refused as well: the value that would be read is not the one the writer wrote
func TestAnExactMemberBesideItsCaseVariantIsRefused(t *testing.T) {
	doc := strings.TrimSuffix(withPins(pendingDoc, pinsA), "}") + `,"PINSSHA256":"` + pinsB + `"}`
	_, err := summarizeStatus(t, doc)
	if _, reason := malformedReason(t, err); reason != memberCaseReason {
		t.Fatalf("reason %q", reason)
	}
}

func TestTheExactSpellingIsStillRead(t *testing.T) {
	s, err := summarizeStatus(t, withPins(pendingDoc, pinsA))
	if err != nil || s.Levels.Finalized.PinsSHA256 != pinsA || s.Levels.Finalized.State != LevelStatePending {
		t.Fatalf("%+v %v", s, err)
	}
}

// the named state is read by "fields"; a member spelled in another case is not that member and is refused
func TestACaseVariantOfAStateMemberIsRefusedByName(t *testing.T) {
	for _, variant := range []string{"Fields", "FIELDS", "fIELDS"} {
		doc := strings.Replace(attemptDoc, `"fields":`, `"`+variant+`":`, 1)
		_, err := summarizeStatus(t, doc)
		if _, reason := malformedReason(t, err); reason != memberCaseReason {
			t.Errorf("fields spelled %s: reason %q", variant, reason)
		}
	}
	for _, variant := range []string{"Code", "CODE"} {
		doc := strings.Replace(attemptDoc, `"code":`, `"`+variant+`":`, 1)
		_, err := summarizeStatus(t, doc)
		if _, reason := malformedReason(t, err); reason != memberCaseReason {
			t.Errorf("code spelled %s: reason %q", variant, reason)
		}
	}
}

// the two members of a state's fields the service validates are read by their exact names; another spelling is refused
func TestACaseVariantOfABackOffFieldIsRefusedByName(t *testing.T) {
	for _, c := range []struct{ member, variant string }{
		{"attempt", "Attempt"}, {"attempt", "ATTEMPT"}, {"next_attempt_at", "Next_Attempt_At"}, {"next_attempt_at", "NEXT_ATTEMPT_AT"},
	} {
		doc := strings.Replace(attemptDoc, `"`+c.member+`":`, `"`+c.variant+`":`, 1)
		_, err := summarizeStatus(t, doc)
		_, reason := malformedReason(t, err)
		if reason != memberCaseReason {
			t.Errorf("%s spelled %s: reason %q", c.member, c.variant, reason)
		}
	}
	// the other fields of a state are free-form names (cause, chain, after, ...): a name that is not one of the two is not a variant of them
	if _, err := summarizeStatus(t, attemptDoc); err != nil {
		t.Fatalf("the exact document: %v", err)
	}
}

// every strict decode in the service is registered here: a new one must be added to this list with its case test, or this test fails
func TestEveryStrictDecodeOfTheServiceIsRegisteredWithItsCaseTest(t *testing.T) {
	registered := map[string]bool{filepath.Join("pkg", "server", "finality_handlers.go"): true}
	root := filepath.Join("..", "..")
	var found []string
	for _, dir := range []string{"pkg", "cmd"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr == nil && strings.Contains(string(b), "DisallowUnknownFields()") {
				rel, _ := filepath.Rel(root, path)
				found = append(found, rel)
			}
			return nil
		})
	}
	for _, f := range found {
		if !registered[f] {
			t.Errorf("%s decodes strictly (DisallowUnknownFields) and has no case-sensitivity test registered here", f)
		}
	}
	if len(found) != len(registered) {
		t.Errorf("strict decodes found %v, registered %v", found, registered)
	}
}

// --- agreement with the validator's writer on golden documents -----------------------------------------------------------------------------

type goldenManifest struct {
	ValidatorCommit string `json:"validatorCommit"`
	Writer          string `json:"writer"`
	Reader          string `json:"reader"`
	Entries         map[string]struct {
		SHA256         string `json:"sha256"`
		ValidatorReads struct {
			Class        string          `json:"class"`
			LevelReached string          `json:"levelReached"`
			NextLevel    string          `json:"nextLevel"`
			ETA          string          `json:"eta"`
			Seq          uint64          `json:"seq"`
			State        json.RawMessage `json:"state"`
			PinsSHA256   string          `json:"pinsSha256"`
		} `json:"validatorReads"`
	} `json:"entries"`
}

// The documents under testdata/finality_status are what the validator's writer wrote (pv2/rb12-train2 d6a9eef1, BuilderStatus.Document), with the
// validator reader's own reading of each in the manifest. The service reads every one, and reads what the validator reads.
func TestTheServiceReadsTheValidatorsGoldenStatusDocumentsAsTheValidatorDoes(t *testing.T) {
	dir := filepath.Join("testdata", "finality_status")
	mb, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m goldenManifest
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	if m.ValidatorCommit != "d6a9eef1" || len(m.Entries) < 9 {
		t.Fatalf("manifest %s with %d entries", m.ValidatorCommit, len(m.Entries))
	}
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	classState := map[string]string{"pending": LevelStatePending, "unavailable": LevelStateUnavailable, "refused": LevelStateRefused, "not_applicable": LevelStateNotApplicable, "verified": LevelStateStoredNoRow}
	for name, e := range m.Entries {
		raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		if err != nil {
			t.Fatal(err)
		}
		if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != e.SHA256 {
			t.Fatalf("%s: the file is not the document the manifest names", name)
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(raw, &members); err != nil {
			t.Fatal(err)
		}
		for k := range members {
			found := false
			for _, c := range contractStatusMembers {
				found = found || c == k
			}
			if !found {
				t.Errorf("%s: the writer spelled a member %q the contract does not list", name, k)
			}
		}
		s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, string(raw))})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		lv := s.Levels.Finalized
		if lv.State == LevelStateNotJudged {
			lv = s.Levels.L1Posted
		}
		v := e.ValidatorReads
		if lv.State != classState[v.Class] || lv.LevelReached != v.LevelReached || lv.NextLevel != v.NextLevel || lv.ETA != v.ETA ||
			uint64(lv.Seq) != v.Seq || lv.PinsSHA256 != v.PinsSHA256 {
			t.Errorf("%s: the service reads %+v, the validator reads %+v", name, lv, v)
		}
		if v.Class == "verified" {
			if len(lv.Reason) != 0 || (len(v.State) != 0 && string(v.State) != "null") {
				t.Errorf("%s: a verified status carries a state", name)
			}
		} else {
			var want bytes.Buffer
			if err := json.Compact(&want, v.State); err != nil || string(lv.Reason) != want.String() {
				t.Errorf("%s: the state the service serves is\n %s\nthe validator's reader returns\n %s", name, lv.Reason, want.String())
			}
		}
	}
}

// RB12-OPS-C item 2 (GW1): the api-gateway reads this very document (its test fixture test/fixtures/finality-summary-pins.json is a
// byte-identical copy), so the shape the gateway surfaces pinsSha256 from is the shape this service serves.
func TestTheSummaryOfARowWithPinsIsTheGatewaysFixtureByteForByte(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	heads := []database.FinalityAddendumHead{headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 47368146, 0x33)}
	statuses := []database.FinalityStatus{statusRow(proof, 84532, 3, withPins(pendingDoc, pinsA))}
	s, err := SummarizeFinality(proof, heads, statuses)
	want, rerr := os.ReadFile(filepath.Join("testdata", "finality_summary", "pins_pending.json"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := summaryJSON(t, s, err); got != string(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}
