package traceutil_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/ctxutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/traceutil"
	"github.com/eluv-io/common-go/util/traceutil/trace"
)

func A() {
	span := traceutil.StartSpan("func A")
	time.Sleep(time.Second)
	defer span.End()
}

func B() {
	span := traceutil.StartSpan("func B")
	time.Sleep(500 * time.Millisecond)
	defer span.End()
}

func Sequential() {
	span := traceutil.StartSpan("calling A() and B() sequentially")
	defer span.End()
	A()
	B()
}

func Parallel() {
	span := traceutil.StartSpan("calling A() and B() concurrently")
	defer span.End()

	wg := sync.WaitGroup{}
	wg.Add(2)

	ctx := ctxutil.Ctx()
	go func() {
		defer ctxutil.Current().Push(ctx)()
		A()
		wg.Done()
	}()
	time.Sleep(50 * time.Millisecond)
	go func() {
		defer ctxutil.Current().Push(ctx)()
		B()
		wg.Done()
	}()

	wg.Wait()
}

func Parallel2() {
	span := traceutil.StartSpan("calling A() and B() concurrently")
	defer span.End()

	wg := sync.WaitGroup{}
	wg.Add(2)

	ctxutil.Current().Go(func() {
		A()
		wg.Done()
	})
	time.Sleep(50 * time.Millisecond)
	ctxutil.Current().Go(func() {
		B()
		wg.Done()
	})

	wg.Wait()
}

func Example() {
	rootSpan := traceutil.InitTracing("root span", false)

	Sequential()
	Parallel()
	Parallel2()

	rootSpan.End()

	// truncate span durations to have reproducible output
	spans := []trace.Span{rootSpan.FindByName("root span")}
	for len(spans) > 0 {
		span := spans[len(spans)-1].(*trace.RecordingSpan)
		span.Data.Duration = duration.Spec(span.Data.Duration.Duration().Truncate(500 * time.Millisecond))
		spans = append(spans[:len(spans)-1], span.Data.Subs...)
	}
	fmt.Println(jsonutil.MustPretty(rootSpan.Json()))

	// Output:
	//
	// {
	//   "name": "root span",
	//   "time": "3.5s",
	//   "subs": [
	//     {
	//       "name": "calling A() and B() sequentially",
	//       "time": "1.5s",
	//       "subs": [
	//         {
	//           "name": "func A",
	//           "time": "1s"
	//         },
	//         {
	//           "name": "func B",
	//           "time": "500ms"
	//         }
	//       ]
	//     },
	//     {
	//       "name": "calling A() and B() concurrently",
	//       "time": "1s",
	//       "subs": [
	//         {
	//           "name": "func A",
	//           "time": "1s"
	//         },
	//         {
	//           "name": "func B",
	//           "time": "500ms"
	//         }
	//       ]
	//     },
	//     {
	//       "name": "calling A() and B() concurrently",
	//       "time": "1s",
	//       "subs": [
	//         {
	//           "name": "func A",
	//           "time": "1s"
	//         },
	//         {
	//           "name": "func B",
	//           "time": "500ms"
	//         }
	//       ]
	//     }
	//   ]
	// }

}
