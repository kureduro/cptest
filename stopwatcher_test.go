package cptest_test

import (
	"testing"
	"time"

	"github.com/kureduro/cptest"
)

func TestSpyStopwatcher(t *testing.T) {

    t.Run("5 calls",
    func(t *testing.T) {
        const totalCalls = 5

        swatch := &cptest.SpyStopwatcher{
            TLAtCall: totalCalls,
        }

        for i := 0; i < totalCalls - 1; i++ {
            select {
            case <-time.After(1 * time.Millisecond):
            case <-swatch.TimeLimit():
                t.Errorf("got TL at call #%d, want at call #%d", i + 1, totalCalls)
            }
        }

        var firstTL, secondTL time.Duration
        select {
        case <-time.After(1 * time.Millisecond):
            t.Fatalf("go no TL at call #%d, want one", totalCalls)
        case firstTL = <-swatch.TimeLimit():
        }

        select {
        case <-time.After(1 * time.Millisecond):
            t.Fatalf("go no TL at call #%d, want one", totalCalls + 1)
        case secondTL = <-swatch.TimeLimit():
        }

        if firstTL != secondTL {
            t.Errorf("got first and seconds TLs that don't match (%v != %v), want matching ones", firstTL, secondTL)
        }

        if firstTL != time.Duration(totalCalls) {
            t.Errorf("got TL equal to %v, want it equal to %v", firstTL, time.Duration(totalCalls))
        }
    })

    t.Run("TL at 0 should never TL",
    func(t *testing.T) {

        swatch := &cptest.SpyStopwatcher{}

        // There won't be more than 10 test cases in the tests, so I think it's
        // enough
        for i := 0; i < 10; i++ {
            select {
            case <-time.After(1 * time.Millisecond):
            case <-swatch.TimeLimit():
                t.Errorf("got TL at call #%d, want none", i + 1)
            }
        }
    })
}

func TestConfigurableStopwatcher(t *testing.T) {

    t.Run("timeout after specified time",
    func(t *testing.T) {

        TL := 7 * time.Millisecond
        timeStep := 2 * time.Millisecond
        steps := int(TL / timeStep / time.Millisecond)

        swatch := cptest.NewConfigurableStopwatcher(TL)

        for i := 0; i < steps; i++ {
            select {
            case <-swatch.TimeLimit():
                curTime := time.Duration(i) * timeStep
                t.Errorf("got timelimit at %v, want at %v", curTime, TL)
            case <-time.After(timeStep):
            }
        }

        time.Sleep(TL - time.Duration(steps) * timeStep)

        eps := 200 * time.Microsecond
        select {
        case realTL := <-swatch.TimeLimit():
            if realTL - TL > eps {
                t.Errorf("received TL with value %v, it deviates from expected %v too much (%v)", 
                    realTL, TL, realTL - TL)
            }
        case <-time.After(timeStep):
            t.Errorf("got no timelimit at %v, want one", TL)
        }
    })

    t.Run("don't timeout if TL less or equal 0",
    func(t *testing.T) {
        swatch := cptest.NewConfigurableStopwatcher(0)

        limit := 20 * time.Millisecond
        step := 1 * time.Millisecond
        for tm := time.Duration(0); tm <= limit; tm += step {
            select {
            case <-swatch.TimeLimit():
                t.Fatalf("got time limit at %v, want none", tm)
            case <-time.After(step):
            }
        }
    })
}