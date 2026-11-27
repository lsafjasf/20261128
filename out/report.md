# Low-frequency routing report

## delay_mismatch.wav

`48000 Hz`, `6 ch`, `6.000 s`, band `20-120 Hz`  
Layout: `FL FR FC LFE BL BR`

| role | ch | label | low RMS dBFS | rel main dB |
|---|---:|---|---:|---:|
| L | 1 | FL | -12.19 | +0.00 |
| R | 2 | FR | -14.14 | -1.95 |
| C | 3 | FC | -59.81 | -47.61 |
| LFE | 4 | LFE | -18.22 | -6.03 |

| pair | zero-lag r | best r | lag ms | gain A->B dB |
|---|---:|---:|---:|---:|
| L/R | +0.000 | -0.001 | - | - |
| L/C | +0.000 | -0.005 | - | - |
| L/LFE | +0.938 | +1.000 | +22.000 | -6.02 |
| R/C | +0.001 | -0.005 | - | - |
| R/LFE | +0.000 | -0.001 | - | - |
| C/LFE | +0.000 | -0.005 | - | - |

**Findings:**
- `delay_mismatch` (warning): LFE is delayed by 22.000 ms (1056 samples at 48000 Hz) relative to L; low-band correlation is +1.000.
- Note: L/LFE is strongly correlated at both zero lag and +22.000 ms; a narrow-band waveform can make lag ambiguous by whole periods.

**Assessment:** A common low-frequency waveform is time-shifted between channels: check routing, alignment, and bass-management bypass.

## inverted.wav

`48000 Hz`, `6 ch`, `6.000 s`, band `20-120 Hz`  
Layout: `FL FR FC LFE BL BR`

| role | ch | label | low RMS dBFS | rel main dB |
|---|---:|---|---:|---:|
| L | 1 | FL | -12.20 | +0.00 |
| R | 2 | FR | -12.20 | +0.00 |
| C | 3 | FC | -12.20 | +0.00 |
| LFE | 4 | LFE | -12.20 | +0.00 |

| pair | zero-lag r | best r | lag ms | gain A->B dB |
|---|---:|---:|---:|---:|
| L/R | +1.000 | +1.000 | +0.000 | +0.00 |
| L/C | -1.000 | -1.000 | +0.000 | +0.00 |
| L/LFE | -1.000 | -1.000 | +0.000 | +0.00 |
| R/C | -1.000 | -1.000 | +0.000 | +0.00 |
| R/LFE | -1.000 | -1.000 | +0.000 | +0.00 |
| C/LFE | +1.000 | +1.000 | +0.000 | +0.00 |

**Findings:**
- `polarity_inversion` (error): C/LFE have opposite polarity to the reference group L/R in the low band (strongest r=-1.000 at +0.000 ms); this is a 180-degree phase/polarity fault candidate.

**Assessment:** A common low-frequency waveform has opposite polarity across channel groups: the listed channels are phase-inverted relative to the reference group.

## lfe_rerouted.wav

`48000 Hz`, `6 ch`, `6.000 s`, band `20-120 Hz`  
Layout: `FL FR FC LFE BL BR`

| role | ch | label | low RMS dBFS | rel main dB |
|---|---:|---|---:|---:|
| L | 1 | FL | -11.38 | +0.00 |
| R | 2 | FR | -11.38 | +0.00 |
| C | 3 | FC | -13.32 | -1.94 |
| LFE | 4 | LFE | -45.36 | -33.98 |

| pair | zero-lag r | best r | lag ms | gain A->B dB |
|---|---:|---:|---:|---:|
| L/R | +1.000 | +1.000 | +0.000 | +0.00 |
| L/C | +1.000 | +1.000 | +0.000 | -1.94 |
| L/LFE | +1.000 | +1.000 | +0.000 | -33.98 |
| R/C | +1.000 | +1.000 | +0.000 | -1.94 |
| R/LFE | +1.000 | +1.000 | +0.000 | -33.98 |
| C/LFE | +1.000 | +1.000 | +0.000 | -32.04 |

**Findings:**
- `lfe_rerouted` (error): LFE is -45.4 dBFS, while its low-frequency waveform appears in L/R/C with up to +34.0 dB makeup gain (r=+1.000 at +0.000 ms).

**Assessment:** The low LFE signal is coherently present in one or more main channels at substantially higher gain: evidence of LFE rerouting or leakage, not normal main-channel music.

## normal_music.wav

`48000 Hz`, `6 ch`, `6.000 s`, band `20-120 Hz`  
Layout: `FL FR FC LFE BL BR`

| role | ch | label | low RMS dBFS | rel main dB |
|---|---:|---|---:|---:|
| L | 1 | FL | -12.20 | +0.00 |
| R | 2 | FR | -13.78 | -1.57 |
| C | 3 | FC | -48.19 | -35.99 |
| LFE | 4 | LFE | -11.38 | +0.82 |

| pair | zero-lag r | best r | lag ms | gain A->B dB |
|---|---:|---:|---:|---:|
| L/R | -0.002 | +0.006 | - | - |
| L/C | +0.000 | -0.014 | - | - |
| L/LFE | -0.001 | -0.003 | - | - |
| R/C | +0.004 | -0.014 | - | - |
| R/LFE | -0.001 | +0.002 | - | - |
| C/LFE | +0.000 | -0.005 | - | - |

**Findings:** none.

**Assessment:** No coherent LFE routing, polarity inversion, or timing mismatch was found in the selected low band.

