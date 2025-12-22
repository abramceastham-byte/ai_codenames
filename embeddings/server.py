"""FastAPI embedding service for Codenames AI."""

from contextlib import asynccontextmanager

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
    max_suggestions: int = 5


class ClueSuggestion(BaseModel):
    clue: str
    score: float
    target_count: int


class SuggestCluesResponse(BaseModel):
    suggestions: list[ClueSuggestion]


class HealthResponse(BaseModel):
    status: str
    model: str


MODEL_NAME = "Qwen/Qwen3-Embedding-0.6B"

model: SentenceTransformer | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global model
    print(f"Loading model {MODEL_NAME}...")
    model = SentenceTransformer(MODEL_NAME)
    print("Model loaded.")
    yield
    model = None


app = FastAPI(title="Codenames Embedding Service", lifespan=lifespan)


@app.get("/health", response_model=HealthResponse)
async def health():
    """Health check endpoint."""
    return HealthResponse(status="ok", model=MODEL_NAME)


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
    """Suggest clues for target words (placeholder - not implemented)."""
    # Placeholder: clue generation is complex and left for future work
    return SuggestCluesResponse(suggestions=[])
