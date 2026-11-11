package mathnote_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"yohaku/internal/mathnote"
)

func TestScalarMathAndCalculatorInput(t *testing.T) {
	e, err := mathnote.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, want string }{
		{"12 x 3", "36"}, {"12x3 + 4", "40"}, {"12×3", "36"}, {"12÷3", "4"},
		{"12X3", "36"}, {"sqrt(-4)", "2i"},
		{"(12 + 3) x -2", "-30"}, {"2^3^2", "512"}, {"2 + 3 x 4", "14"},
		{"x = 7\ntax = 12\nmax(tax, x)\nx + 1", "8"},
		{"x3 = 9\nx3 + 1", "10"}, {"0xff + 1", "256"}, {"1e3x2", "2000"},
		{"price = 19\nquantity = 3\nprice x quantity", "57"},
		{"1336\n23 /\n# x × in a comment\nmissing\nprev + 1", "1337"},
		{"min = ones\nmin(2)", "2"}, {"x = 4\nx = 1/0\nx + 1", "5"},
		{"13 inches in cm", "33.02 cm"}, {"sqrt(81) + abs(-2)", "11"},
		{"sin(90 deg)", "1"}, {"round(0.1 + 0.2, 2)", "0.3"},
		{"1,234.50 + 2,000.25", "3234.75"}, {"(1,234 + 6) / 10", "124"},
		{"max(1,234)", "234"}, {"min(3, 12)", "3"},
		{"卫生纸 111 + 222", "333"}, {"1 月消费 1000 + 2000", "3000"},
		{"退款 -111 + 222", "111"},
		{"x1 = 1 月消费 1000 + 2000\nx2 = 2 月消费 1000 + 3000\n合计 x1 + x2 = ", "7000"},
		{"x = 7\nx 2", "14"}, {"total 1,234 + 6 = 1240", "1240"},
		{"13473.05 + 845.96", "14319.01"}, {"0.1 + 0.2 - 0.3", "0"},
		{"x1=0.1+0.2\nx1-0.3", "0"}, {"x1=90\nsin(x1 deg)", "1"},
		{"2+3=5", "5"}, {"2+3=99", "5"}, {"price=7\nprice = 9 =", "9"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			r, err := e.Evaluate(context.Background(), []mathnote.Block{{Start: 17, Text: tc.source}})
			if err != nil || len(r) == 0 || r[len(r)-1].Copy != tc.want || r[len(r)-1].Error != "" {
				t.Fatalf("want %q; results=%+v err=%v", tc.want, r, err)
			}
			if r[len(r)-1].End != 17+len(tc.source) {
				t.Fatal("result lost original UTF-8 offset")
			}
		})
	}
	r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: "x=9"}, {Start: 100, Text: "x + 1\nprev + 1\n2+3"}})
	if err != nil || len(r) != 4 || r[1].Error == "" || r[2].Error == "" || r[3].Copy != "5" {
		t.Fatal("scope leaked between blocks", r, err)
	}
}

func TestIncompleteInputIsQuietButInvalidExpressionsRemainVisible(t *testing.T) {
	e, err := mathnote.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"4/", "2 + ", "12*", "12 x", "12x", "2 X # still typing", "1e-", "1e+", "sqrt(", "sqrt(9", "(2+3", "price =", "2 to", "2+ # still typing"} {
		t.Run(source, func(t *testing.T) {
			// Neither a partial assignment nor a partial expression changes
			// the last successful value or leaves a stale copyable result.
			r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: "price=7\n" + source + "\nprev + price"}})
			if err != nil || len(r) != 2 || r[0].Copy != "7" || r[1].Copy != "14" || r[1].Error != "" {
				t.Fatalf("partial input produced a result/error or changed scope: %+v %v", r, err)
			}
		})
	}
	for _, source := range []string{"2+*3", "1+)", "sin(,2)", "(1 + *", "1e-+", "1/0", "unknown + 1", "2 + unknown", "1 + * missing", "12,34 + 1", "1,234,56", "x1234=9\nx1,234", "2 unknown + 1", "卫生纸 111 + * 222", "2 == 2"} {
		r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: source}})
		if err != nil || len(r) == 0 || r[len(r)-1].Error == "" || r[len(r)-1].Copy != "" {
			t.Fatalf("invalid expression hidden: %q %+v %v", source, r, err)
		}
	}
}

func TestProseAndBareAmountsAreQuiet(t *testing.T) {
	e, err := mathnote.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"房贷", "信用卡", "合计", "当前余额", "monthly budget", "14 号",
		"招行 13,473.05", "工行 845.96", "公积金提款 8724.81", "工行 -845.96", "余额 +0",
		"11-14 提前还款 91846.38", "11-22 正常还款 商贷待定，公积金 3600",
		"13473.05", "-845.96", "(8724.81)", "1e3", "1,234 # 金额",
	} {
		r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: source}})
		if err != nil || len(r) != 0 {
			t.Errorf("non-formula %q produced hints: %+v %v", source, r, err)
		}
	}
	// Quiet literal rows retain their existing prev semantics; a note does
	// not overwrite it. Explicit assignments and '=' still ask for a result.
	r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: "100\n备注\nprev + 7\nx1 = 20\n招行 30 ="}})
	if err != nil || len(r) != 3 || r[0].Copy != "107" || r[1].Copy != "20" || r[2].Copy != "30" || !r[2].Complete {
		t.Fatalf("quiet rows changed calculation semantics: %+v %v", r, err)
	}
}

func TestGroupedDisplayKeepsCopyAndComputationUnchanged(t *testing.T) {
	e, err := mathnote.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, display, copy string }{
		{"999 + 0", "999", "999"}, {"999 + 1", "1,000", "1000"},
		{"13473.05 + 845.96", "14,319.01", "14319.01"},
		{"-1234567.8901 + 0", "-1,234,567.8901", "-1234567.8901"},
		{"1000000 / 8", "125,000", "125000"}, {"0.0001234 + 0", "0.0001234", "0.0001234"},
		{"1e14 + 0", "1e+14", "1e+14"},
		{"12345 cm in cm", "12,345 cm", "12345 cm"},
		{"1000 + 2000i", "1000 + 2000i", "1000 + 2000i"},
		{"x1 = 1234.56\nx1 + 0.44", "1,235", "1235"},
		{"1234.5 =", "1,234.5", "1234.5"}, {"1234.5 = 1234.5", "1,234.5", "1234.5"},
	} {
		r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: tc.source}})
		if err != nil || len(r) == 0 || r[len(r)-1].Display != tc.display || r[len(r)-1].Copy != tc.copy {
			t.Fatalf("%q: expected display %q / copy %q, got %+v %v", tc.source, tc.display, tc.copy, r, err)
		}
	}
}

func TestResultTriggerDoesNotConfuseAssignmentsOrComparisons(t *testing.T) {
	e, err := mathnote.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source, want      string
		complete, written bool
	}{
		{"2+3 = ", "5", true, false}, {"x1=2+3=", "5", true, false},
		{"x1=2+3", "5", false, false}, {"2+3=5", "5", false, true},
		{"2+3=99", "5", false, false}, {"x1=2+3=5", "5", false, true},
		{"2+3 # =", "5", false, false}, {"2+3 = # note", "5", false, false},
		{"x1=", "", false, false}, {"2+/=", "", false, false},
	} {
		r, err := e.Evaluate(context.Background(), []mathnote.Block{{Start: 17, Text: tc.source}})
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == "" {
			for _, v := range r {
				if v.Copy != "" || v.Complete {
					t.Fatalf("bad completion: %+v", r)
				}
			}
		} else if len(r) != 1 || r[0].Copy != tc.want || r[0].Display != tc.want || r[0].Complete != tc.complete || r[0].Written != tc.written || r[0].End != 17+len(tc.source) {
			t.Fatalf("%q: %+v", tc.source, r)
		}
	}
}

func TestMathLimitsAndRecovery(t *testing.T) {
	e, err := mathnote.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"[1, 2]", "1:100000000", "ones(1000000)", "f(x)=f(x)", "2; 3", "a.b", "\"12x3\"", "import(\"x\")", "1/0"} {
		r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: source}})
		if err != nil || len(r) != 1 || r[0].Error == "" || r[0].Copy != "" {
			t.Fatalf("unsafe expression accepted %q: %+v %v", source, r, err)
		}
	}
	// A too-deep expression may hit the VM stack limit before AST traversal;
	// either rejection must return no successful result and leave it reusable.
	r, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: strings.Repeat("1+", 300) + "1"}})
	if err == nil && (len(r) != 1 || r[0].Error == "" || r[0].Copy != "") {
		t.Fatal("complexity limit ignored", r)
	}
	if _, err := e.Evaluate(context.Background(), []mathnote.Block{{Text: strings.Repeat("1\n", 2001)}}); err == nil {
		t.Fatal("missing note limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Evaluate(ctx, []mathnote.Block{{Text: "1+2"}}); err == nil {
		t.Fatal("cancel ignored")
	}
	// Exercise interruption inside real evaluation, not just an already
	// cancelled request, then use the same VM again below.
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, err = e.Evaluate(ctx, []mathnote.Block{{Text: strings.Repeat("sqrt(81)+sin(90 deg)\n", 1800)}})
	cancel()
	if err == nil {
		t.Fatal("running calculation ignored deadline")
	}
	r, err = e.Evaluate(context.Background(), []mathnote.Block{{Text: "12 x 3"}})
	if err != nil || len(r) != 1 || r[0].Copy != "36" {
		t.Fatal("cancel poisoned next job", r, err)
	}
}

func TestWorkerLatestSnapshotAndClose(t *testing.T) {
	w := mathnote.NewWorker()
	defer w.Close()
	w.Submit(1, []mathnote.Block{{Text: "old = 9"}})
	w.Submit(2, []mathnote.Block{{Text: "old + 1\n12x3"}})
	deadline := time.After(10 * time.Second)
	for {
		select {
		case r := <-w.Results:
			if r.Generation != 2 {
				continue
			}
			if r.Err != nil || len(r.Results) != 2 || r.Results[0].Error == "" || r.Results[1].Copy != "36" {
				t.Fatal("stale scope/snapshot", r)
			}
			return
		case <-deadline:
			t.Fatal("worker did not finish")
		}
	}
}
