package nft

import (
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
)

// FuzzRenderRule drives the renderer with synthesised expression lists.
//
// RenderRule is the only piece of the read path that walks attacker-
// influenced data (the kernel-supplied expr.Any slice). A bug that
// panics or eats unbounded memory there crashes the TUI on the
// operator's screen — fuzz it to catch the unusual combinations that
// hand-crafted tests miss.
//
// Run locally:
//
//	go test -fuzz=FuzzRenderRule -fuzztime=30s ./internal/nft/
//
// Under regular `go test ./...` the function only replays the seed
// corpus, which is enough to keep the well-known nft patterns
// regression-safe.
func FuzzRenderRule(f *testing.F) {
	// Seed corpus — each is a (opcodes, data) pair the harness
	// decodes into an expression list. The opcodes byte stream is a
	// sequence of "next expression type" selectors; data feeds their
	// payloads. The four seeds below cover the common shapes the
	// hand-written tests in render_test.go exercise.
	f.Add([]byte{}, []byte{})                                                                                                                                                         // empty rule
	f.Add([]byte{0x00, 0x02, 0x07}, []byte{0x01, 0x01, 0x00, 'l', 'o', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})                                                                        // iif+cmp+verdict
	f.Add([]byte{0x01, 0x02, 0x07}, []byte{0x01, 0x01, 0x0c, 0x04, 0x00, 0x01, 0x04, 0x0a, 0x00, 0x00, 0x00, 0x00})                                                                   // ip saddr cmp verdict
	f.Add([]byte{0x02, 0x03, 0x02, 0x07}, []byte{0x01, 0x01, 0x09, 0x01, 0x01, 0x04, 0x06, 0x00, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x01, 0x00, 0x16, 0x00}) // ct + bitwise + cmp + verdict

	f.Fuzz(func(t *testing.T, opcodes, data []byte) {
		// Cap the number of expressions and the per-expr data slice
		// to keep one fuzz iteration bounded. Without this the fuzzer
		// will eventually find an input that allocates gigabytes of
		// state and hangs.
		if len(opcodes) > 256 {
			opcodes = opcodes[:256]
		}
		if len(data) > 4096 {
			data = data[:4096]
		}

		rule := &nftables.Rule{Exprs: buildSyntheticExprs(opcodes, data)}
		out := RenderRule(rule, nil)

		// Sanity invariants. NFT is allowed to be empty (an empty rule
		// has no expressions) but must never contain a NUL byte —
		// we'd be silently embedding raw network data into a string
		// that downstream callers paste into nft scripts.
		for i := 0; i < len(out.NFT); i++ {
			if out.NFT[i] == 0 {
				t.Fatalf("rendered NFT contains NUL byte at offset %d: %q", i, out.NFT)
			}
		}
	})
}

// buildSyntheticExprs decodes (opcodes, data) into a sequence of
// expr.Any values. The opcode byte modulo the dispatch-table size
// picks the type; each handler pops as many bytes from data as it
// needs. Out-of-data short-circuits return zeros so the renderer
// sees plausibly-shaped (if nonsensical) input.
func buildSyntheticExprs(opcodes, data []byte) []expr.Any {
	var out []expr.Any
	di := 0
	popByte := func() byte {
		if di >= len(data) {
			return 0
		}
		b := data[di]
		di++
		return b
	}
	popBytes := func(n int) []byte {
		if n < 0 {
			n = 0
		}
		if n > 32 { // cap any single field at 32 bytes
			n = 32
		}
		if di+n > len(data) {
			n = len(data) - di
			if n < 0 {
				n = 0
			}
		}
		b := data[di : di+n]
		di += n
		return b
	}
	popReg := func() uint32 {
		// nftables registers live in [1..31]. Mask to that range so
		// we don't waste fuzz cycles on the renderer's
		// "unknown register" placeholder branch (already covered by
		// hand-written tests).
		return uint32(popByte()%31) + 1
	}

	for _, op := range opcodes {
		switch op % 13 {
		case 0:
			out = append(out, &expr.Meta{
				Key:      expr.MetaKey(popByte() % 32),
				Register: popReg(),
			})
		case 1:
			out = append(out, &expr.Payload{
				DestRegister: popReg(),
				Base:         expr.PayloadBase(popByte() % 3),
				Offset:       uint32(popByte()),
				Len:          uint32(popByte() % 17),
			})
		case 2:
			ln := int(popByte() % 17)
			out = append(out, &expr.Cmp{
				Op:       expr.CmpOp(popByte() % 6),
				Register: popReg(),
				Data:     popBytes(ln),
			})
		case 3:
			ln := int(popByte() % 17)
			out = append(out, &expr.Bitwise{
				SourceRegister: popReg(),
				DestRegister:   popReg(),
				Len:            uint32(ln),
				Mask:           popBytes(ln),
				Xor:            popBytes(ln),
			})
		case 4:
			out = append(out, &expr.Ct{
				Key:      expr.CtKey(popByte() % 32),
				Register: popReg(),
			})
		case 5:
			out = append(out, &expr.Lookup{
				SourceRegister: popReg(),
				DestRegister:   popReg(),
				SetName:        string(popBytes(int(popByte() % 16))),
				Invert:         popByte()%2 == 0,
			})
		case 6:
			out = append(out, &expr.Counter{
				Packets: uint64(popByte()) << 8,
				Bytes:   uint64(popByte()) << 12,
			})
		case 7:
			kinds := []expr.VerdictKind{
				expr.VerdictAccept, expr.VerdictDrop, expr.VerdictReturn,
				expr.VerdictJump, expr.VerdictGoto, expr.VerdictContinue,
				expr.VerdictQueue, expr.VerdictStop,
			}
			k := kinds[int(popByte())%len(kinds)]
			chainNameLen := int(popByte() % 16)
			out = append(out, &expr.Verdict{
				Kind:  k,
				Chain: string(popBytes(chainNameLen)),
			})
		case 8:
			out = append(out, &expr.Log{
				Data: popBytes(int(popByte() % 16)),
			})
		case 9:
			out = append(out, &expr.Reject{
				Type: uint32(popByte() % 3),
				Code: popByte(),
			})
		case 10:
			out = append(out, &expr.Limit{
				Rate: uint64(popByte()),
				Unit: expr.LimitTime(popByte() % 5),
			})
		case 11:
			out = append(out, &expr.Masq{})
		case 12:
			out = append(out, &expr.Immediate{})
		}
	}
	return out
}
