package cptest_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/kuredoro/cptest"
	"github.com/sanity-io/litter"
)

func ProcFuncMultiply(ctx context.Context, in io.Reader) (cptest.ProcessResult, error) {
	var a, b int
	fmt.Fscan(in, &a, &b)

	return cptest.ProcessResult{
		ExitCode: 0,
		Stdout:   fmt.Sprintln(a * b),
		Stderr:   "",
	}, nil
}

func ProcFuncIntegerSequence(ctx context.Context, in io.Reader) (cptest.ProcessResult, error) {
	var n int
	fmt.Fscan(in, &n)

	buf := &bytes.Buffer{}
	for i := 1; i <= n; i++ {
		fmt.Fprint(buf, i, " ")
	}

	fmt.Fprintln(buf)

	return cptest.ProcessResult{
		ExitCode: 0,
		Stdout:   buf.String(),
		Stderr:   "",
	}, nil
}

func ProcFuncBogusFloatingPoint(ctx context.Context, in io.Reader) (cptest.ProcessResult, error) {
	var n int
	fmt.Fscan(in, &n)

	out := ""
	if n == 1 {
		out = "1.234567\n"
	} else if n == 2 {
		out = "2.345678\n"
	}

	return cptest.ProcessResult{
		ExitCode: 0,
		Stdout:   out,
		Stderr:   "",
	}, nil
}

func ProcFuncAnswer(ctx context.Context, in io.Reader) (cptest.ProcessResult, error) {
	return cptest.ProcessResult{
		ExitCode: 0,
		Stdout:   "42",
		Stderr:   "",
	}, nil
}

func TestNewTestingBatch(t *testing.T) {
	t.Run("no state altering configs", func(t *testing.T) {
		inputs := cptest.Inputs{
			Tests:  nil,
			Config: map[string]string{},
		}

		batch := cptest.NewTestingBatch(inputs, nil, nil, nil)

		if batch.Lx.Precision != cptest.DefaultPrecision {
			t.Errorf("got lexer precision %d, but want default value %d",
				batch.Lx.Precision, cptest.DefaultPrecision)
		}
	})

	t.Run("prec option", func(t *testing.T) {
		inputs := cptest.Inputs{
			Tests: nil,
			Config: map[string]string{
				"prec": "22",
			},
		}

		batch := cptest.NewTestingBatch(inputs, nil, nil, nil)

		if batch.Lx.Precision != 22 {
			t.Errorf("got lexer precision %d, but want 22", batch.Lx.Precision)
		}
	})
}

// IDEA: Add support for presentation errors...
func TestTestingBatch(t *testing.T) {
	t.Run("all OK", func(t *testing.T) {
		inputs := cptest.Inputs{
			Tests: []cptest.Test{
				{
					Input:  "2 2\n",
					Output: "4\n",
				},
				{
					Input:  "-2 -2\n",
					Output: "4\n",
				},
			},
		}

		proc := &cptest.SpyProcesser{
			Proc: cptest.ProcesserFunc(ProcFuncMultiply),
		}

		swatch := &cptest.ConfigurableStopwatcher{Clock: clockwork.NewFakeClock()}
		pool := cptest.NewSpyThreadPool(2)

		batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)
		batch.Run()

		want := map[int]cptest.Verdict{
			1: cptest.OK,
			2: cptest.OK,
		}

		cptest.AssertVerdicts(t, batch.Verdicts, want)
		cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 2)
		cptest.AssertThreadCount(t, pool, 2)
	})

	t.Run("outputs are compared lexeme-wise", func(t *testing.T) {
		inputs := cptest.Inputs{
			Tests: []cptest.Test{
				{
					Input:  "2\n",
					Output: "1  2\n",
				},
				{
					Input:  "3\n",
					Output: "1  2  3\n",
				},
			},
		}

		proc := &cptest.SpyProcesser{
			Proc: cptest.ProcesserFunc(ProcFuncIntegerSequence),
		}

		swatch := &cptest.ConfigurableStopwatcher{Clock: clockwork.NewFakeClock()}
		pool := cptest.NewSpyThreadPool(2)

		batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)
		batch.Run()

		want := map[int]cptest.Verdict{
			1: cptest.OK,
			2: cptest.OK,
		}

		cptest.AssertVerdicts(t, batch.Verdicts, want)
		cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 2)
		cptest.AssertThreadCount(t, pool, 2)

		if len(batch.RichAnswers[1]) != 3 || len(batch.RichAnswers[2]) != 4 {
			t.Errorf("got wrong rich answers, %s", litter.Sdump(batch.RichAnswers))
		}

		if len(batch.RichOuts[1]) != 3 || len(batch.RichOuts[2]) != 4 {
			t.Errorf("got wrong rich outputs, %s", litter.Sdump(batch.RichOuts))
		}
	})

	t.Run("floating point values are compared correctly", func(t *testing.T) {
		inputs := cptest.Inputs{
			Tests: []cptest.Test{
				{
					Input:  "1\n",
					Output: "1.25\n",
				},
				{
					Input:  "2\n",
					Output: "2.5\n",
				},
			},
			Config: map[string]string{
				"prec": "1",
			},
		}

		proc := &cptest.SpyProcesser{
			Proc: cptest.ProcesserFunc(ProcFuncBogusFloatingPoint),
		}

		swatch := &cptest.ConfigurableStopwatcher{Clock: clockwork.NewFakeClock()}
		pool := cptest.NewSpyThreadPool(2)

		batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)
		batch.Run()

		want := map[int]cptest.Verdict{
			1: cptest.OK,
			2: cptest.WA,
		}

		cptest.AssertVerdicts(t, batch.Verdicts, want)
		cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 2)
		cptest.AssertThreadCount(t, pool, 2)
	})

	t.Run("all WA",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{
						Input:  "4\n1 2 3 4\n",
						Output: "2 3 4 5\n",
					},
					{
						Input:  "2\n-2 -1\n",
						Output: "-1 0\n",
					},
				},
			}

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(ProcFuncAnswer),
			}

			swatch := &cptest.ConfigurableStopwatcher{Clock: clockwork.NewFakeClock()}
			pool := cptest.NewSpyThreadPool(2)

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)
			batch.Run()

			want := map[int]cptest.Verdict{
				1: cptest.WA,
				2: cptest.WA,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, want)
			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 2)
			cptest.AssertThreadCount(t, pool, 2)
		})

	t.Run("runtime error and internal error",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{
						Input:  "1\n",
						Output: "1\n",
					},
					{
						Input:  "2\n",
						Output: "2\n",
					},
					{
						Input:  "3\n",
						Output: "3\n",
					},
					{
						Input:  "4\n",
						Output: "4\n",
					},
					{
						Input:  "5\n",
						Output: "5\n",
					},
				},
			}

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(
					func(ctx context.Context, r io.Reader) (cptest.ProcessResult, error) {
						var num int
						fmt.Fscan(r, &num)

						if num == 3 {
							return cptest.ProcessResult{
								ExitCode: 1,
								Stdout:   "",
								Stderr:   "segfault. (core dumped)",
							}, nil
						}

						if num == 5 {
							panic("brrrr")
						}

						return cptest.ProcessResult{
							ExitCode: 0,
							Stdout:   "1\n",
							Stderr:   "",
						}, nil
					}),
			}

			swatch := &cptest.ConfigurableStopwatcher{Clock: clockwork.NewFakeClock()}
			pool := cptest.NewSpyThreadPool(3)

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)
			batch.Run()

			want := map[int]cptest.Verdict{
				1: cptest.OK,
				2: cptest.WA,
				3: cptest.RE,
				4: cptest.WA,
				5: cptest.IE,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, want)
			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 5)
			cptest.AssertThreadCount(t, pool, 3)

			if len(batch.RichAnswers[3]) == 0 || len(batch.RichAnswers[5]) == 0 {
				t.Errorf("got wrong rich answers, %s", litter.Sdump(batch.RichAnswers))
			}
		})

	t.Run("single TL (proc doesn't run because it didn't have time to dispatch)",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{"\n", "bar\n"},
				},
			}

			clock := clockwork.NewFakeClock()

			killCount := 0

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(
					func(ctx context.Context, r io.Reader) (cptest.ProcessResult, error) {
						<-ctx.Done()
						killCount++

						return cptest.ProcessResult{
							ExitCode: 0,
							Stdout:   "",
							Stderr:   "",
						}, nil
					}),
			}

			swatch := &cptest.ConfigurableStopwatcher{
				Clock: clock,
				TL:    3 * time.Second,
			}
			pool := cptest.NewSpyThreadPool(1)

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				batch.Run()
				wg.Done()
			}()

			clock.BlockUntil(1)
			clock.Advance(3 * time.Second)

			wg.Wait()

			testsWant := map[int]cptest.Verdict{
				1: cptest.TL,
			}

			timesWant := map[int]time.Duration{
				1: 3 * time.Second,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, testsWant)
			cptest.AssertThreadCount(t, pool, 1)

			// Should be too fast for anyone to be killed.
			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 0)
			cptest.AssertCallCount(t, "process cancel", killCount, 0)
			cptest.AssertTimes(t, batch.Times, timesWant)
		})

	t.Run("single TL (proc runs)",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{"\n", "bar\n"},
				},
			}

			clock := clockwork.NewFakeClock()

			killCount := 0

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(
					func(ctx context.Context, r io.Reader) (cptest.ProcessResult, error) {
						select {
						case <-clock.After(5 * time.Second):
						case <-ctx.Done():
							killCount++
							return cptest.ProcessResult{
								ExitCode: 0,
								Stdout:   "",
								Stderr:   "",
							}, cptest.TLError
						}

						return cptest.ProcessResult{
							ExitCode: 0,
							Stdout:   "",
							Stderr:   "",
						}, nil

					}),
			}

			swatch := &cptest.ConfigurableStopwatcher{
				Clock: clock,
				TL:    3 * time.Second,
			}
			pool := cptest.NewSpyThreadPool(1)

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				batch.Run()
				wg.Done()
			}()

			clock.BlockUntil(2)
			clock.Advance(3 * time.Second)

			wg.Wait()

			testsWant := map[int]cptest.Verdict{
				1: cptest.TL,
			}

			timesWant := map[int]time.Duration{
				1: 3 * time.Second,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, testsWant)
			cptest.AssertThreadCount(t, pool, 1)

			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 1)
			cptest.AssertCallCount(t, "process cancel", killCount, 1)
			cptest.AssertTimes(t, batch.Times, timesWant)
		})

	t.Run("two TL, thread count 1",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{"\n", "bar\n"},
					{"\n", "bar\n"},
				},
			}

			clock := clockwork.NewFakeClock()

			killCount := 0

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(
					func(ctx context.Context, r io.Reader) (cptest.ProcessResult, error) {
						select {
						case <-clock.After(5 * time.Second):
						case <-ctx.Done():
							killCount++
							return cptest.ProcessResult{
								ExitCode: 0,
								Stdout:   "",
								Stderr:   "",
							}, cptest.TLError
						}

						return cptest.ProcessResult{
							ExitCode: 0,
							Stdout:   "",
							Stderr:   "",
						}, nil

					}),
			}

			swatch := &cptest.ConfigurableStopwatcher{
				Clock: clock,
				TL:    3 * time.Second,
			}
			pool := cptest.NewSpyThreadPool(1)

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				batch.Run()
				wg.Done()
			}()

			clock.BlockUntil(2)
			clock.Advance(3 * time.Second)
			clock.BlockUntil(3)
			clock.Advance(3 * time.Second)

			wg.Wait()

			testsWant := map[int]cptest.Verdict{
				1: cptest.TL,
				2: cptest.TL,
			}

			timesWant := map[int]time.Duration{
				1: 3 * time.Second,
				2: 3 * time.Second,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, testsWant)
			cptest.AssertThreadCount(t, pool, 1)

			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 2)
			cptest.AssertCallCount(t, "process cancel", killCount, 2)
			cptest.AssertTimes(t, batch.Times, timesWant)
		})

	t.Run("two TL, thread count 2",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{"\n", "bar\n"},
					{"\n", "bar\n"},
				},
			}

			clock := clockwork.NewFakeClock()

			var mu sync.Mutex
			killCount := 0

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(
					func(ctx context.Context, r io.Reader) (cptest.ProcessResult, error) {
						select {
						case <-clock.After(5 * time.Second):
						case <-ctx.Done():
							mu.Lock()
							killCount++
							mu.Unlock()
							return cptest.ProcessResult{
								ExitCode: 0,
								Stdout:   "",
								Stderr:   "",
							}, cptest.TLError
						}

						return cptest.ProcessResult{
							ExitCode: 0,
							Stdout:   "",
							Stderr:   "",
						}, nil

					}),
			}

			swatch := &cptest.ConfigurableStopwatcher{
				Clock: clock,
				TL:    3 * time.Second,
			}
			pool := cptest.NewSpyThreadPool(2)

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				batch.Run()
				wg.Done()
			}()

			clock.BlockUntil(3)
			clock.Advance(3 * time.Second)

			wg.Wait()

			testsWant := map[int]cptest.Verdict{
				1: cptest.TL,
				2: cptest.TL,
			}

			timesWant := map[int]time.Duration{
				1: 3 * time.Second,
				2: 3 * time.Second,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, testsWant)
			cptest.AssertThreadCount(t, pool, 2)

			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 2)
			cptest.AssertCallCount(t, "process cancel", killCount, 2)
			cptest.AssertTimes(t, batch.Times, timesWant)
		})

	t.Run("two TL two OK, thread count 2",
		func(t *testing.T) {
			inputs := cptest.Inputs{
				Tests: []cptest.Test{
					{"2\n", "2\n"},
					{"5\n", "5\n"},
					{"2\n", "2\n"},
					{"5\n", "5\n"},
				},
			}

			clock := clockwork.NewFakeClock()

			var mu sync.Mutex
			killCount := 0

			proc := &cptest.SpyProcesser{
				Proc: cptest.ProcesserFunc(
					func(ctx context.Context, r io.Reader) (cptest.ProcessResult, error) {
						line, _ := ioutil.ReadAll(r)

						var num int
						num, err := strconv.Atoi(string(line[:len(line)-1]))

						if err != nil {
							panic(err)
						}

						select {
						case <-clock.After(time.Duration(num) * time.Second):
						case <-ctx.Done():
							mu.Lock()
							killCount++
							mu.Unlock()
							return cptest.ProcessResult{
								ExitCode: 0,
								Stdout:   "",
								Stderr:   "",
							}, nil
						}

						return cptest.ProcessResult{
							ExitCode: 0,
							Stdout:   string(line),
							Stderr:   "",
						}, nil

					}),
			}

			swatch := &cptest.ConfigurableStopwatcher{
				Clock: clock,
				TL:    3 * time.Second,
			}
			pool := cptest.NewSpyThreadPool(2)

			doneCh := make(chan struct{}, 1)
			done := func(b *cptest.TestingBatch, t cptest.Test, id int) {
				doneCh <- (struct{}{})
			}

			batch := cptest.NewTestingBatch(inputs, proc, swatch, pool)
			batch.TestEndCallback = done

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				batch.Run()
				wg.Done()
			}()

			// Time:   12345
			// test 1: --
			// test 2: ---
			// test 3:   --
			// test 4:    ---
			clock.BlockUntil(3)
			clock.Advance(2 * time.Second)

			advances := []time.Duration{time.Second, time.Second, 2 * time.Second}
			blocks := []int{4, 5, 5}
			for i := range advances {
				<-doneCh
				clock.BlockUntil(blocks[i])

				clock.Advance(advances[i])
				if i == 2 {
                    time.Sleep(200 * time.Millisecond)
					break
				}
			}

			wg.Wait()

			testsWant := map[int]cptest.Verdict{
				1: cptest.OK,
				2: cptest.TL,
				3: cptest.OK,
				4: cptest.TL,
			}

			timesWant := map[int]time.Duration{
				1: 2 * time.Second,
				2: 3 * time.Second,
				3: 2 * time.Second,
				4: 3 * time.Second,
			}

			cptest.AssertVerdicts(t, batch.Verdicts, testsWant)
			cptest.AssertThreadCount(t, pool, 2)

			cptest.AssertCallCount(t, "proc.Run()", proc.CallCount(), 4)
			cptest.AssertCallCount(t, "process cancel", killCount, 2)
			cptest.AssertTimes(t, batch.Times, timesWant)
		})
}
