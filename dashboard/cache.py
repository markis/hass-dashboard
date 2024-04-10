import pickle
from datetime import datetime
from pathlib import Path
from typing import Final, Generic, TypeVar

from dashboard.calendar import TZ

T = TypeVar("T")


class Cache(Generic[T]):
    path: Final[Path]
    expiration: Final[int]

    def __init__(self, path: Path, expiration: int) -> None:
        self.path = path
        self.expiration = expiration

    def load_cache(self) -> T | None:
        try:
            last_modified = self.path.stat().st_mtime
            now = datetime.now(tz=TZ).timestamp()
            if self.path.exists() and last_modified + self.expiration > now:
                with self.path.open("rb") as f:
                    data: T = pickle.load(f)
                    return data
        except (FileNotFoundError, pickle.UnpicklingError):
            pass
        return None

    def save_cache(self, data: T) -> None:
        with self.path.open("wb") as f:
            pickle.dump(data, f)
