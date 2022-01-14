# UTC - A `time.Time` Wrapper for ISO8601 / RFC3339 

The package `eluv-io/utc` consists of a single struct `UTC` that is a small wrapper around the standard lib's `time.Time`. It provides the following main functions:

* times in the UTC timezone (Coordinated Universal Time)
* consistent formatting and parsing of the [ISO8601](https://en.wikipedia.org/wiki/ISO_8601) / [RFC3339](https://tools.ietf.org/html/rfc3339) format '2006-01-02T15:04:05.000Z'
* performance-optimized string formatting, binary marshal/unmarshal
* mocking of "Now()" for tests


## Usage

Use just like time.Time:

```go
func ExampleUTC() {
    // standard lib time.Time
    location, _ := time.LoadLocation("Indian/Mayotte")
    d0std := time.Date(2000, 1, 1, 0, 0, 0, 0, location)
    fmt.Println("d0 std ", d0std)
    
    // utc.UTC
    d0 := utc.New(d0std)
    fmt.Println("d0     ", d0.String())
    
    // utc.UTC.Time is the underlying time.Time (in UTC timezone)
    fmt.Println("d0.Time", d0.Time)
    
    // ISO8601 / RFC3339
    d1 := utc.MustParse("2021-12-25T12:20:00.000Z")
    fmt.Println("d1     ", d1)
    
    // All methods of time.Time are available on utc.UTC
    fmt.Println("d1-d0  ", d1.Sub(d0))
    
    // JSON and text marshalling produce & parse ISO8601 / RFC3339
    jsn, _ := d1.MarshalText()
    fmt.Println("d1     ", string(jsn), "MarshalText()")
    
    // Output:
    //
    // d0 std  2000-01-01 00:00:00 +0300 EAT
    // d0      1999-12-31T21:00:00.000Z
    // d0.Time 1999-12-31 21:00:00 +0000 UTC
    // d1      2021-12-25T12:20:00.000Z
    // d1-d0   192711h20m0s
    // d1      2021-12-25T12:20:00.000Z MarshalText()
}
```

Mocking Now():

```go
func ExampleMockNowFn() {
    d0 := utc.MustParse("2020-01-01T00:00:00.000Z")
    now := d0.Add(38*time.Hour + 30*time.Minute)

    // replace Now() with a custom function that provides the mocked time
    reset := utc.MockNowFn(func() utc.UTC {
    return now
    })
    defer reset()
    
    fmt.Println("now   ", utc.Now(), "mocked")
    time.Sleep(1 * time.Second)
    fmt.Println("now   ", utc.Now(), "one real second later: still the same")
    fmt.Println("now-d0", utc.Now().Sub(d0))
    
    now = now.Add(time.Second)
    fmt.Println("now   ", utc.Now(), "one mocked second later")
    
    // Output:
    //
    // now    2020-01-02T14:30:00.000Z mocked
    // now    2020-01-02T14:30:00.000Z one real second later: still the same
    // now-d0 38h30m0s
    // now    2020-01-02T14:30:01.000Z one mocked second later
}
```
