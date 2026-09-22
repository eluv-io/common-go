# fMP4 test fixtures

- `vinit-stream0.m4s`, `vchunk-stream0-00001.m4s`: AVC init segment and one media segment, copied from
  content-fabric's `extapi/avtest/encrypt/testdata`.
- `hevc-init.m4s`: HEVC init segment, same origin. Used to check that unsupported codecs are rejected.
- `ainit.m4s`, `achunk-00001.m4s`, `achunk-00002.m4s`: AAC-LC init segment and two media segments (48 kHz stereo,
  one fragment per frame), generated with

  ```
  ffmpeg -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=2" -c:a aac -b:a 96k -ar 48000 -ac 2 \
    -f dash -seg_duration 1 -frag_type every_frame -use_template 1 -use_timeline 0 \
    -init_seg_name ainit.m4s -media_seg_name 'achunk-$Number%05d$.m4s' audio.mpd
  ```

  and keeping the first two segments only.
