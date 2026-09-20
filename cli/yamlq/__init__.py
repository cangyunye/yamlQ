import importlib.metadata

from yamlq.parser import load_views, ViewConfig
from yamlq.gateway import Gateway
from yamlq.renderer import render_table

try:
    from yamlq._commit import __build_version__
except ImportError:
    __build_version__ = ""

# build 版本号(task install-python 写入,git describe 的 tag 版本)优先,
# 未生成 _commit.py 时回退到包元数据版本
if __build_version__:
    __version__ = __build_version__
else:
    try:
        __version__ = importlib.metadata.version("yamlq")
    except importlib.metadata.PackageNotFoundError:
        __version__ = "dev"

try:
    from yamlq._commit import __commit__
except ImportError:
    __commit__ = "none"

try:
    from yamlq._commit import __built_at__
except ImportError:
    __built_at__ = "unknown"

__all__ = ["load_views", "ViewConfig", "Gateway", "render_table", "__version__", "__build_version__", "__commit__", "__built_at__"]
