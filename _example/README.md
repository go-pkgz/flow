# flow example

This toy example demonstrates a typical use of flow library.
The goal is to read input files from the `data `directory and process their content concurrently. The processing
includes filtering of short words and calculating of the total size and number of passed words. It also calculates how many times word "data" appeared.

`go run example.go`

output:

```
2018/11/09 13:58:00 flow example started
2018/11/09 13:58:00 make words handler with minsize=3
2018/11/09 13:58:00 make line split handler
2018/11/09 13:58:00 make line split handler
2018/11/09 13:58:00 start words handler 0 with minsize=3
2018/11/09 13:58:00 start words handler 1 with minsize=3
2018/11/09 13:58:00 start words handler 2 with minsize=3
2018/11/09 13:58:00 start words handler 3 with minsize=3
2018/11/09 13:58:00 start words handler 4 with minsize=3
2018/11/09 13:58:00 start words handler 5 with minsize=3
2018/11/09 13:58:00 start words handler 6 with minsize=3
2018/11/09 13:58:00 start words handler 7 with minsize=3
2018/11/09 13:58:00 start words handler 8 with minsize=3
2018/11/09 13:58:00 start words handler 9 with minsize=3
2018/11/09 13:58:00 start sum handler
2018/11/09 13:58:00 start line split handler 0
2018/11/09 13:58:00 start line split handler 1
2018/11/09 13:58:00 file reader completed for data/input-1.txt, read 35 lines (total 35)
2018/11/09 13:58:00 file reader completed for data/input-3.txt, read 27 lines (total 62)
2018/11/09 13:58:00 file reader completed for data/input-4.txt, read 5 lines (total 67)
2018/11/09 13:58:00 line split handler completed, id=0 lines=67
2018/11/09 13:58:00 file reader completed for data/input-2.txt, read 70 lines (total 137)
2018/11/09 13:58:00 line split handler completed, id=1 lines=137
2018/11/09 13:58:00 words handler completed, id=005 147/848 words
2018/11/09 13:58:00 words handler completed, id=004 368/368 words
2018/11/09 13:58:00 words handler completed, id=007 333/701 words
2018/11/09 13:58:00 words handler completed, id=002 748/1596 words
2018/11/09 13:58:00 words handler completed, id=000 453/2049 words
2018/11/09 13:58:00 words handler completed, id=009 439/2488 words
2018/11/09 13:58:00 words handler completed, id=001 317/2805 words
2018/11/09 13:58:00 words handler completed, id=003 348/3153 words
2018/11/09 13:58:00 words handler completed, id=008 111/3264 words
2018/11/09 13:58:00 words handler completed, id=006 120/3384 words
2018/11/09 13:58:00 final result: words=3384, size=21393, spec=34
2018/11/09 13:58:00 flow example finished with err=<nil>
```