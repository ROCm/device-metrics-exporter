"""Configuration file for the Sphinx documentation builder."""

external_projects_remote_repository = "https://raw.githubusercontent.com/ROCm/dcgpu-docs/refs/heads/develop/docs/projects.yaml"
external_projects_current_project = "device-metrics-exporter"

project = "AMD Instinct Documentation"
version = "1.0.0"
release = version
html_title = f"Device Metrics Exporter {version}"
author = "Advanced Micro Devices, Inc."
copyright = "Copyright (c) 2024 Advanced Micro Devices, Inc. All rights reserved."

# Required settings
html_theme = "rocm_docs_theme"
html_theme_options = {
    "flavor": "generic",
    "header_title": "AMD Instinct Documentation",
    "nav_secondary_items": {
        "GitHub": "https://github.com/rocm/device-metrics-exporter",
        "Documentation": "https://dcgpu.docs.amd.com"
    },
    # Add any additional theme options here
}
extensions = ["rocm_docs"]

# Table of contents
external_toc_path = "./sphinx/_toc.yml"

exclude_patterns = ['.venv']