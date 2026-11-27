# 24-bit to 16-bit Transient Audit

- Input: `out/source-24bit.wav`
- Headroom target: -1.00 dBFS
- Applied headroom gain: 0.944061 (-0.500 dB)

| method | src peak dBFS | out peak dBFS | peak Δ dB | transient E Δ dB | attack Δ dB | residual RMS LSB | residual peak LSB | transient residual RMS LSB | transient residual peak LSB | corr | repeat corr | HPF residual LSB | clips |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| truncate | -0.500 | -0.500 | -0.0003 | -0.0000 | -0.0001 | 0.5749 | 0.9961 | 0.5790 | 0.9961 | -0.0007 | 1.0000 | 0.2551 | 0 |
| tpdf_dither | -0.500 | -0.500 | 0.0000 | 0.0000 | 0.0001 | 0.5006 | 1.4531 | 0.4968 | 1.3867 | -0.0002 | 0.0043 | 0.4425 | 0 |
| shaped_tpdf | -0.500 | -0.500 | -0.0003 | 0.0000 | -0.0004 | 0.8124 | 2.6602 | 0.8104 | 2.6602 | 0.0003 | -0.0043 | 0.7388 | 0 |
| corrected_shaped_tpdf | -1.000 | -1.000 | -0.0002 | 0.0000 | 0.0000 | 0.8159 | 2.7852 | 0.8110 | 2.3906 | 0.0004 | -0.0023 | 0.7421 | 0 |

Interpretation:
- Truncation has repeat correlation of 1: the same input always produces the same error, even when its instantaneous source correlation is low.
- TPDF dither decorrelates independent quantization runs; its residual RMS is close to the ideal 1/sqrt(12) LSB, while transient energy and peak remain statistically unbiased.
- Shaped TPDF moves residual energy above the low-frequency band; inspect HPF residual together with residual peak because noise shaping can create short overshoots.
- The corrected path reserves headroom before shaped TPDF quantization, preventing full-scale clamps without adding a limiter or altering transient timing.
- Per-frame CSV files show residual envelopes inside and outside transient windows; do not judge the result using a static spectrum alone.
