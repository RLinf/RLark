from setuptools import setup, find_packages

setup(
    name="embodied-runtime",
    version="0.1.0",
    description="Python SDK for the embodied-runtime robot & camera gRPC services",
    long_description=open("README.md", encoding="utf-8").read(),
    long_description_content_type="text/markdown",
    author="rlinf",
    author_email="pigeonligh@hotmail.com",
    url="https://github.com/RLinf/RLark/tree/main/sdks/embodied-runtime-python",
    license="Apache 2.0",
    packages=find_packages(include=["embodied_runtime", "embodied_runtime.*"]),
    include_package_data=True,
    python_requires=">=3.10",
    install_requires=[
        # Lower bounds are dictated by the checked-in stubs under
        # embodied_runtime/gen; see requirements.txt.
        "grpcio>=1.84.0",
        "protobuf>=7.35.1,<8.0.0",
    ],
    extras_require={
        "dev": [
            # Pinned, see requirements-dev.txt.
            "grpcio-tools==1.84.0",
            "pytest>=7.0",
            "pytest-cov>=4.0",
        ],
    },
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "License :: OSI Approved :: Apache Software License",
        "Operating System :: POSIX :: Linux",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Programming Language :: Python :: 3.13",
        "Topic :: Software Development :: Libraries :: Python Modules",
    ],
)
