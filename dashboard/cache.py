from datetime import datetime, timedelta
from typing import Final, Generic, TypeVar

from dashboard.calendar import TZ

T = TypeVar("T")


class Cache(Generic[T]):
    _cache: dict[str, tuple[T, datetime]]
    expiration: Final[int]

    def __init__(self, expiration: int) -> None:
        self._cache = {}
        self.expiration = expiration

    def load(self, key: str) -> T | None:
        if key in self._cache:
            data, last_modified = self._cache[key]
            now = datetime.now(tz=TZ)
            if last_modified + timedelta(seconds=self.expiration) > now:
                return data

            del self._cache[key]
        return None

    def save(self, key: str, data: T) -> None:
        self._cache[key] = (data, datetime.now(tz=TZ))
