import asyncio

from dashboard.generate import generate_image


async def generate_image_every_minute() -> None:
    while True:
        await asyncio.gather(asyncio.sleep(60), generate_image())


def run() -> None:
    asyncio.run(generate_image_every_minute())


if __name__ == "__main__":
    run()
