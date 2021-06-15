# M3U8 loader

This util downloads all .ts segments from specified URL and then joins them in one .mp4 file using ffmpeg.

ffmpeg is required for this downloader.

## Usage 

```
m3u8-loader <m3u8-url> <output file>
```

Example:

```
m3u8-loader https://multiplatform-f.akamaihd.net/i/multi/will/bunny/big_buck_bunny_,640x360_400,640x360_700,640x360_1000,950x540_1500,.f4v.csmil/master.m3u8 out.mp4
```

