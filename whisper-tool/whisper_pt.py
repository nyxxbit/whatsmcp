#!/usr/bin/env python
"""Shared Whisper configuration: model, cache and CUDA.

The model is loaded once and kept in a module-level cache. Before this, every
caller built its own WhisperModel, and the batch transcriber reloaded the model
for each file, so a run of 46 voice notes paid the load cost 46 times.

Two decisions worth keeping, both measured on 524s of real voice notes:

1. large-v3-turbo instead of large-v3: 63.6s -> 17.3s, a 3.7x speedup (not the
   8x advertised), with practically identical text. Override with
   WHISPER_MODEL=large-v3.

2. condition_on_previous_text=False. With it on, the model occasionally locks
   into a loop and repeats a phrase until the segment ends.

DO NOT use initial_prompt or hotwords. Both were tested and rejected. The idea
was to feed domain vocabulary so the model would misspell fewer proper nouns.
Measured against plain turbo on the same 8 files:

  - long initial_prompt: swallowed a negation in 2 files, which inverts the
    meaning of the sentence, and injected a word from the vocabulary into an
    unrelated transcript.
  - short initial_prompt (names only): still swallowed a negation.
  - hotwords: worst of the three. 58.8% similarity against the baseline, wrote a
    vocabulary term into 3 files that never contained it, and ran slower.

The problem this was meant to solve did not exist: the proper nouns were already
being transcribed correctly. Losing a negation costs far more than misspelling a
name, so prompt-based steering is not worth it.

If a term genuinely needs fixing, substitute it AFTER transcription. That is
deterministic and cannot invent text. See CORRECTIONS below.
"""
import glob
import importlib.util
import os
import re

MODEL = os.environ.get("WHISPER_MODEL", "large-v3-turbo")
DEVICE = os.environ.get("WHISPER_DEVICE", "cuda")
COMPUTE = os.environ.get("WHISPER_COMPUTE", "float16")
LANGUAGE = os.environ.get("WHISPER_LANG", "pt")
CPU_COMPUTE = os.environ.get("WHISPER_CPU_COMPUTE", "int8")


def register_cuda_dlls():
    """Put the pip-installed CUDA runtime on the Windows DLL search path.

    The nvidia-*-cu12 wheels drop their DLLs in site-packages/nvidia/<pkg>/bin,
    a directory Windows never searches. ctranslate2 then fails with "Library
    cublas64_12.dll is not found or cannot be loaded" while the file sits right
    there, and the whole transcription run dies on the first audio. Registering
    the directories here, before ctranslate2 is imported, is what makes the GPU
    reachable. Returns the directories it registered.
    """
    if os.name != "nt":
        return []
    roots = []
    spec = importlib.util.find_spec("nvidia")
    for base in list(getattr(spec, "submodule_search_locations", None) or []):
        roots.extend(sorted(glob.glob(os.path.join(base, "*", "bin"))))
    spec = importlib.util.find_spec("ctranslate2")
    if spec and spec.origin:
        roots.append(os.path.dirname(spec.origin))
    roots = [d for d in roots if os.path.isdir(d)]
    for d in roots:
        try:
            os.add_dll_directory(d)
        except OSError:
            pass
    # add_dll_directory only covers loads that go through LoadLibraryEx with
    # altered search flags. Some ctranslate2 builds resolve their dependencies
    # the plain way, which reads PATH, so cover that too.
    if roots:
        os.environ["PATH"] = os.pathsep.join(roots) + os.pathsep + os.environ.get("PATH", "")
    return roots


register_cuda_dlls()

# Post-transcription fixes: regex -> replacement. Applied to the final text, so
# they cannot cause the model to hallucinate. Add domain terms here rather than
# passing them to the model as a prompt.
CORRECTIONS = {}

_model = None


def get_model(log=None):
    """Return the cached model, loading it on first use.

    Falls back to the CPU when the GPU cannot be initialised. A batch of voice
    notes taking longer beats a batch that dies on file 1 because a CUDA
    library moved, which is exactly what happened on 17/08/2026.
    """
    global _model, DEVICE, COMPUTE
    if _model is None:
        from faster_whisper import WhisperModel
        if log:
            log(f"loading {MODEL} on {DEVICE} ({COMPUTE})")
        try:
            _model = WhisperModel(MODEL, device=DEVICE, compute_type=COMPUTE)
        except Exception as exc:
            if DEVICE == "cpu":
                raise
            DEVICE, COMPUTE = "cpu", CPU_COMPUTE
            msg = f"GPU unavailable ({exc}); falling back to {DEVICE} ({COMPUTE})"
            log(msg) if log else print(msg, flush=True)
            _model = WhisperModel(MODEL, device=DEVICE, compute_type=COMPUTE)
    return _model


def unload():
    """Drop the model and free VRAM."""
    global _model
    _model = None
    try:
        import torch
        torch.cuda.empty_cache()
    except Exception:
        pass


def correct(text):
    for pattern, replacement in CORRECTIONS.items():
        text = re.sub(pattern, replacement, text, flags=re.IGNORECASE)
    return text


def transcribe(path, log=None):
    """Transcribe one audio file and return the full text."""
    model = get_model(log=log)
    segments, _ = model.transcribe(
        path,
        language=LANGUAGE,
        beam_size=5,
        vad_filter=True,
        condition_on_previous_text=False,
    )
    return correct(" ".join(s.text.strip() for s in segments).strip())
