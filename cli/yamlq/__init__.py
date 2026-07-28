import importlib.metadata

from yamlq.parser import load_views, ViewConfig
from yamlq.gateway import Gateway
from yamlq.renderer import render_table

try:
    __version__ = importlib.metadata.version("yamlq")
except importlib.metadata.PackageNotFoundError:
    __version__ = "dev"

try:
    from yamlq._commit import __commit__
except ImportError:
    __commit__ = "none"

__all__ = ["load_views", "ViewConfig", "Gateway", "render_table", "__version__", "__commit__"]
