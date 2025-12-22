"""FastAPI embedding service for Codenames AI."""

import os
from contextlib import asynccontextmanager
from pathlib import Path

import numpy as np
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer


class SimilarityRequest(BaseModel):
    clue: str
    candidates: list[str]


class WordScore(BaseModel):
    word: str
    similarity: float


class SimilarityResponse(BaseModel):
    scores: list[WordScore]


class SuggestCluesRequest(BaseModel):
    targets: list[str]
    avoid: list[str]
    board_words: list[str]  # All words on the board (for filtering)
    max_suggestions: int = 5


class ClueSuggestion(BaseModel):
    clue: str
    score: float
    target_scores: list[float]  # Similarity to each target


class SuggestCluesResponse(BaseModel):
    suggestions: list[ClueSuggestion]


class HealthResponse(BaseModel):
    status: str
    model: str
    vocab_size: int


MODEL_NAME = "Qwen/Qwen3-Embedding-0.6B"
VOCAB_PATH = os.environ.get("VOCAB_PATH", "data/common_words.txt")

model: SentenceTransformer | None = None
vocab_words: list[str] = []
vocab_embeddings: np.ndarray | None = None


def load_vocabulary(path: str) -> list[str]:
    """Load vocabulary from a newline-delimited file."""
    vocab_file = Path(path)
    if not vocab_file.exists():
        print(f"Warning: Vocabulary file {path} not found, clue suggestions disabled")
        return []

    words = []
    with open(vocab_file) as f:
        for line in f:
            word = line.strip().lower()
            if word and len(word) > 1:  # Skip empty lines and single chars
                words.append(word)

    return words


@asynccontextmanager
async def lifespan(app: FastAPI):
    global model, vocab_words, vocab_embeddings

    print(f"Loading model {MODEL_NAME}...")
    model = SentenceTransformer(MODEL_NAME)
    print("Model loaded.")

    print(f"Loading vocabulary from {VOCAB_PATH}...")
    vocab_words = load_vocabulary(VOCAB_PATH)
    print(f"Loaded {len(vocab_words)} vocabulary words.")

    if vocab_words and model:
        print("Pre-computing vocabulary embeddings...")
        vocab_embeddings = model.encode(vocab_words, show_progress_bar=True)
        print(f"Vocabulary embeddings shape: {vocab_embeddings.shape}")

    yield

    model = None
    vocab_words = []
    vocab_embeddings = None


app = FastAPI(title="Codenames Embedding Service", lifespan=lifespan)


def is_too_close_to_board_word(clue: str, board_words: list[str]) -> bool:
    """Check if clue is a substring of or contains any board word."""
    clue_lower = clue.lower()
    for board_word in board_words:
        board_lower = board_word.lower().replace("_", "")
        if clue_lower in board_lower or board_lower in clue_lower:
            return True
    return False


@app.get("/health", response_model=HealthResponse)
async def health():
    """Health check endpoint."""
    return HealthResponse(
        status="ok",
        model=MODEL_NAME,
        vocab_size=len(vocab_words),
    )


@app.post("/similarity", response_model=SimilarityResponse)
async def compute_similarity(req: SimilarityRequest):
    """Compute cosine similarity between a clue word and candidate words."""
    if model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    if not req.candidates:
        raise HTTPException(status_code=400, detail="candidates list is empty")

    if not req.clue:
        raise HTTPException(status_code=400, detail="clue is empty")

    clue_embedding = model.encode([req.clue])
    candidate_embeddings = model.encode(req.candidates)

    similarities = model.similarity(clue_embedding, candidate_embeddings)[0]

    scores = [
        WordScore(word=word, similarity=float(sim))
        for word, sim in zip(req.candidates, similarities)
    ]
    scores.sort(key=lambda x: x.similarity, reverse=True)

    return SimilarityResponse(scores=scores)


@app.post("/suggest-clues", response_model=SuggestCluesResponse)
async def suggest_clues(req: SuggestCluesRequest):
    """Suggest clues for target words.

    Scoring: score = avg(similarity to targets) - max(similarity to avoid words)

    Higher scores mean the clue is:
    - Similar to all target words (what we want to hint at)
    - Dissimilar to avoid words (opponent's words, bystanders, assassin)
    """
    if model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    if vocab_embeddings is None or len(vocab_words) == 0:
        raise HTTPException(status_code=503, detail="Vocabulary not loaded")

    if not req.targets:
        raise HTTPException(status_code=400, detail="targets list is empty")

    # Encode target and avoid words
    target_embeddings = model.encode(req.targets)
    avoid_embeddings = model.encode(req.avoid) if req.avoid else None

    # Compute similarities: vocab_embeddings (V x D) @ target_embeddings.T (D x T) = (V x T)
    # model.similarity returns a torch tensor, convert to numpy
    target_sims = model.similarity(vocab_embeddings, target_embeddings).cpu().numpy()  # (V x T)

    if avoid_embeddings is not None:
        avoid_sims = model.similarity(vocab_embeddings, avoid_embeddings).cpu().numpy()  # (V x A)
        max_avoid_sim = np.max(avoid_sims, axis=1)  # (V,)
    else:
        max_avoid_sim = np.zeros(len(vocab_words))

    # Score = avg(target_sim) - max(avoid_sim)
    avg_target_sim = np.mean(target_sims, axis=1)  # (V,)
    scores = avg_target_sim - max_avoid_sim  # (V,)

    # Get top candidates (more than needed, we'll filter)
    top_k = min(req.max_suggestions * 10, len(vocab_words))
    top_indices = np.argsort(scores)[::-1][:top_k]

    # Filter and build response
    suggestions = []
    board_words_lower = [w.lower() for w in req.board_words]

    for idx in top_indices:
        if len(suggestions) >= req.max_suggestions:
            break

        word = vocab_words[idx]

        # Skip if too close to any board word
        if is_too_close_to_board_word(word, req.board_words):
            continue

        # Skip if the word is in the board words
        if word.lower() in board_words_lower:
            continue

        suggestions.append(
            ClueSuggestion(
                clue=word,
                score=float(scores[idx]),
                target_scores=[float(s) for s in target_sims[idx]],
            )
        )

    return SuggestCluesResponse(suggestions=suggestions)
