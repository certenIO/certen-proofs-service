#!/usr/bin/env python3
"""Writes the golden `finality` members and golden composed documents of this directory from chains.json.

chains.json is the validator's own vector set finality_chain2 (certenIO/certen-validator, origin/pv2/rb11-b2b2a-db, file
pkg/finality/testdata/finality_chain2/chains.json, sha256 fe430d589f48013e6deb4ffba1be85247b4685433e138c614b9b94f438237b08),
copied byte for byte. The member text is written here WITHOUT any Go code, member by member in the order of the validator's
MarshalJSON (addendum.go: Section{format, leg{chain, blockNumber, blockHash}, addenda[]}; addendum{proofId, seq, level, chain,
kind, evidence, prevHash, ownHash}), so the Go composition is compared with an independent statement of the form, not with itself.

Usage: python gen_goldens.py   (run from this directory)
"""
import hashlib
import json

SRC = "chains.json"
raw = open(SRC, "rb").read()
assert hashlib.sha256(raw).hexdigest() == "fe430d589f48013e6deb4ffba1be85247b4685433e138c614b9b94f438237b08", "chains.json changed"
vectors = {v["name"]: v for v in json.loads(raw)["vectors"]}


def compact(x):
    return json.dumps(x, separators=(",", ":"), ensure_ascii=False)


ADDENDUM_ORDER = ["proofId", "seq", "level", "chain", "kind", "evidence", "prevHash", "ownHash"]


def member(v):
    adds = v["addenda"]
    leg = v["leg"]
    legtxt = '{"chain":%d,"blockNumber":%d,"blockHash":%s}' % (leg["chain"], leg["blockNumber"], compact(leg["blockHash"]))
    addtxt = ",".join("{" + ",".join('"%s":%s' % (k, compact(a[k])) for k in ADDENDUM_ORDER) + "}" for a in adds)
    return '{"format":"certen-finality-addenda/1","leg":%s,"addenda":[%s]}' % (legtxt, addtxt)


STORED = {
    "format": "certen-proof-v2-accumulate-portable/1",
    "pin": "aa",
    "genesis": {"x": 1},
    "majors": [],
    "evidence": {"version": "2.0"},
}
MAJORS = [{"index": 1}, {"index": 2}]
GOV = {"g0Hash": "00"}


def document(finality):
    members = {k: v for k, v in STORED.items()}
    members["majors"] = MAJORS
    members["govRootV3Inputs"] = GOV
    if finality is not None:
        members["finality"] = json.loads(finality)
    # Go marshals a map with its keys sorted bytewise; nested values keep their own order (json.RawMessage).
    parts = []
    for k in sorted(members):
        if k == "finality":
            parts.append('"finality":' + finality)
        else:
            parts.append('"%s":%s' % (k, compact(members[k])))
    return "{" + ",".join(parts) + "}"


for name in ["valid_sepolia_finalized", "valid_base_l1_posted_then_finalized", "valid_base_finalized_then_l1_posted"]:
    m = member(vectors[name])
    open("section_%s.golden.json" % name, "w", encoding="utf-8", newline="\n").write(m)
    open("document_%s.golden.json" % name, "w", encoding="utf-8", newline="\n").write(document(m))
open("document_valid_empty.golden.json", "w", encoding="utf-8", newline="\n").write(document(None))
