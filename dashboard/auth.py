import aiohttp


class BearerAuth(aiohttp.BasicAuth):
    def __init__(self, token: str):
        self.token = token

    def encode(self) -> str:
        return f"Bearer {self.token}"
