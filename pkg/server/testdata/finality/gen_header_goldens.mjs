// Writes the two section goldens that carry leg.headerRlp (R12-OUTCOME-TX) from legs_with_header.json (real headers, dumped from the
// validator's fixtures) by assembling every byte by hand: node gen_header_goldens.mjs <this directory>. The rows are those of the
// Go tests (row(): prevHash = seq repeated, ownHash = seq+0x10 repeated; evidence = the CFE1 header + 0x61).
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
const dir = process.argv[2];
const H = JSON.parse(readFileSync(join(dir, 'legs_with_header.json'), 'utf8')).headers;
const PROOF = '01234567-89ab-4def-8012-3456789abcde';
const u = (n, bytes) => { let s = ''; for (let i = bytes - 1; i >= 0; i--) s += Number((BigInt(n) >> BigInt(8 * i)) & 0xffn).toString(16).padStart(2, '0'); return s; };
const ascii = (s) => Buffer.from(s).toString('hex');
const cfe1 = (kind, chain, level, number, hash) => ascii('CFE1') + u(kind.length, 4) + ascii(kind) + u(chain, 8) + u(level, 1) + u(number, 8) + hash + '61';
const rep = (b) => u(b, 1).repeat(32);
function section(leg, kinds) {
  const rows = kinds.map(([seq, level, name, kind]) => ({
    proofId: PROOF, seq, level: name, chain: leg.chain, kind,
    evidence: cfe1(kind, leg.chain, level, leg.number, leg.hash), prevHash: rep(seq), ownHash: rep(seq + 0x10),
  }));
  return JSON.stringify({ format: 'certen-finality-addenda/1', leg: { chain: leg.chain, blockNumber: leg.number, blockHash: leg.hash, headerRlp: leg.headerRlp }, addenda: rows });
}
const b = H.base_sepolia_47368146, a = H.telcoin_adiri_498759;
writeFileSync(join(dir, 'section_header_bundle_base_47368146.golden.json'), section(b, [[1, 2, 'l1_posted', 'base-sepolia/l1_posted/v1'], [2, 3, 'finalized', 'base-sepolia/finalized/v1']]));
writeFileSync(join(dir, 'section_header_legacy_adiri_498759.golden.json'), section(a, [[1, 3, 'finalized', 'adiri/finalized/v1']]));
