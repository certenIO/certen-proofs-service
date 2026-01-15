PS C:\Users\jason> cd C:\Accumulate_Stuff\certen\certen-protocol\services\validator
PS C:\Accumulate_Stuff\certen\certen-protocol\services\validator> $env:GOWORK = "off"
PS C:\Accumulate_Stuff\certen\certen-protocol\services\validator> go test -tags performance ./tests/performance/... -v
=== RUN   TestLoadBundleGeneration
    benchmark_test.go:494: Load Test Results (Bundle Generation):
    benchmark_test.go:495:   Total Requests:      597669
    benchmark_test.go:496:   Successful:          597669
    benchmark_test.go:497:   Failed:              0
    benchmark_test.go:498:   Requests/Second:     33165.24
    benchmark_test.go:499:   P50 Latency:         0.00 ms
    benchmark_test.go:500:   P95 Latency:         3.54 ms
    benchmark_test.go:501:   P99 Latency:         35.96 ms
--- PASS: TestLoadBundleGeneration (129.27s)
=== RUN   TestLoadSignatureVerification
    benchmark_test.go:538: Load Test Results (Signature Verification):
    benchmark_test.go:539:   Total Verifications: 1646591
    benchmark_test.go:540:   Verifications/Sec:   54854.16
    benchmark_test.go:541:   P99 Latency:         3.00 ms
--- PASS: TestLoadSignatureVerification (463.59s)
=== RUN   TestLoadMerkleVerification
panic: test timed out after 10m0s
        running tests:
                TestLoadMerkleVerification (7s)

goroutine 239 [running]:
testing.(*M).startAlarm.func1()
        C:/Program Files/Go/src/testing/testing.go:2682 +0x345
created by time.goFunc
        C:/Program Files/Go/src/time/sleep.go:215 +0x2d

goroutine 1 [chan receive, 1 minutes]:
testing.(*T).Run(0xc000003500, {0x7ff677e6b21d?, 0x7ff8414b3fb0?}, 0x7ff677e752d0)
        C:/Program Files/Go/src/testing/testing.go:2005 +0x469
testing.runTests.func1(0xc000003500)
        C:/Program Files/Go/src/testing/testing.go:2477 +0x37
testing.tRunner(0xc000003500, 0xc000133c70)
        C:/Program Files/Go/src/testing/testing.go:1934 +0xc3
testing.runTests(0xc000008198, {0x7ff677fe5f80, 0x4, 0x4}, {0x7?, 0xc0000728c0?, 0x7ff677feb7c0?})
        C:/Program Files/Go/src/testing/testing.go:2475 +0x4b4
testing.(*M).Run(0xc000076320)
        C:/Program Files/Go/src/testing/testing.go:2337 +0x63a
main.main()
        _testmain.go:73 +0x9b

goroutine 60 [sync.WaitGroup.Wait, 1 minutes]:
sync.runtime_SemacquireWaitGroup(0xc0001277a0?, 0x80?)
        C:/Program Files/Go/src/runtime/sema.go:114 +0x2e
sync.(*WaitGroup).Wait(0xc003b44090)
        C:/Program Files/Go/src/sync/waitgroup.go:206 +0x85
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest({0x7ff677eae280, 0xc0001260e0}, 0x32, 0xc00070b920)
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:435 +0x296
github.com/certen/certen-protocol/services/validator/tests/performance.TestLoadMerkleVerification(0xc0003868c0)
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:584 +0x3cb
testing.tRunner(0xc0003868c0, 0x7ff677e752d0)
        C:/Program Files/Go/src/testing/testing.go:1934 +0xc3
created by testing.(*T).Run in goroutine 1
        C:/Program Files/Go/src/testing/testing.go:1997 +0x44b

goroutine 203 [runnable]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 197 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 202 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 201 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 194 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 200 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 204 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 63 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 198 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 196 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 65 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 62 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 195 [runnable]:
crypto/internal/fips140/sha256.(*Digest).checkSum(0xc000281ca0?)
        C:/Program Files/Go/src/crypto/internal/fips140/sha256/sha256.go:225 +0x6f
crypto/internal/fips140/sha256.(*Digest).Sum(0xc000281d80, {0xc000281d60, 0x0, 0x20})
        C:/Program Files/Go/src/crypto/internal/fips140/sha256/sha256.go:204 +0x7d
crypto/sha256.Sum256({0xc0065b2800, 0x40, 0x40})
        C:/Program Files/Go/src/crypto/sha256/sha256.go:60 +0xc5
github.com/certen/certen-protocol/services/validator/tests/performance.TestLoadMerkleVerification.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:593 +0x85
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:400 +0x119
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 64 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 199 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 61 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 205 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 206 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 207 [runnable]:
github.com/certen/certen-protocol/services/validator/tests/performance.TestLoadMerkleVerification.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:589 +0x131
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:400 +0x119
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 208 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 209 [runnable]:
internal/sync.runtime_Semrelease(0xbb076b79?, 0x0?, 0x0?)
        C:/Program Files/Go/src/runtime/sema.go:124 +0x13
internal/sync.(*Mutex).unlockSlow(0xc003b44058?, 0xeeef3c51?)
        C:/Program Files/Go/src/internal/sync/mutex.go:221 +0x9b
internal/sync.(*Mutex).Unlock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:198
sync.(*Mutex).Unlock(...)
        C:/Program Files/Go/src/sync/mutex.go:65
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:429 +0x2f0
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 210 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 211 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 212 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 213 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 214 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 215 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 216 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 217 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 218 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 219 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 220 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 221 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 222 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 223 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 224 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 225 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 226 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 227 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 228 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 229 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 230 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 231 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 232 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 233 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 234 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 235 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 236 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 237 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157

goroutine 238 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x389bf644e25b54?, 0x14?, 0x40?)
        C:/Program Files/Go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0xc003b44058)
        C:/Program Files/Go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        C:/Program Files/Go/src/sync/mutex.go:46
github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest.func1()
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:427 +0x1df
created by github.com/certen/certen-protocol/services/validator/tests/performance.runLoadTest in goroutine 60
        C:/Accumulate_Stuff/certen/certen-protocol/services/validator/tests/performance/benchmark_test.go:391 +0x157
FAIL    github.com/certen/certen-protocol/services/validator/tests/performance  600.732s
FAIL
PS C:\Accumulate_Stuff\certen\certen-protocol\services\validator>