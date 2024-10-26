# Contents of bazel/rite_go_generated_srcs.bzl:

load(
    "@rules_go//go:def.bzl",
    "GoLibrary",
    "GoSource",
    "go_context")
load(
    "@rules_go//proto:compiler.bzl",
    "GoProtoCompiler",
)

def _go_library_srcs_impl(ctx):

    #output_file = ctx.actions.declare_file(ctx.label.name + ".tar")
    output_file = ctx.outputs.out
    spec_file = ctx.actions.declare_file(ctx.label.name + ".packager_Spec.json")
    spec, all_files = _generate_input_spec(ctx)
    ctx.actions.write(spec_file, json.encode_indent(spec))

    ctx.actions.run(
        mnemonic = "PackageSourcesIntoTar",
        executable = ctx.executable._packager,
        arguments = [
            "--alsologtostderr",
            "--output",
            output_file.path,
            "--spec",
            spec_file.path,
        ],
        inputs = [
            spec_file,
        ] + all_files,
        outputs = [output_file],
    )

    return [
        DefaultInfo(files = depset([output_file])),
    ]


def _generate_input_spec(ctx):
    go = go_context(ctx)

    all_src_files = []
    importpath_to_src_files = {}
    for src in ctx.attr.deps:
        lib = src[GoLibrary]
        go_src = go.library_to_source(go, ctx.attr, lib, False)
        
        importpath = lib.importpath
        if importpath not in importpath_to_src_files:
            importpath_to_src_files[importpath] = []
        importpath_to_src_files[importpath].extend(go_src.srcs)
        all_src_files.extend(go_src.srcs)

    print(importpath_to_src_files)

    return (
        struct(
            packages = [
                struct(
                    import_path = key,
                    files = [_file_to_dict(f) for f in files]
                )
                for (key, files) in importpath_to_src_files.items()
            ]),
            all_src_files
    )

def _file_to_dict(file):
    return {
        "path": file.path,
        "is_directory": file.is_directory,
        "is_source": file.is_source,
        "short_path": file.short_path,
        "root": file.root.path,
        "owner": str(file.owner),
    }

output_go_library_srcs = rule(
    implementation = _go_library_srcs_impl,
    attrs = {
        "deps": attr.label_list(
            providers = [GoLibrary],
            aspects = [],
        ),
        "out": attr.output(
          doc = ("Name of output .tar file. If not specified, the file name " +
          "of the generated source file will be used."),
          mandatory = False,
        ),
        "_packager": attr.label(
            default = Label("//internal/cmd/source-archiver"),
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        # Needed for go_context to work.. see
        # https://github.com/bazel-contrib/rules_go/blob/master/go/toolchains.rst#id12
        "compiler": attr.label(
            providers = [GoProtoCompiler],
            default = "@rules_go//proto:go_proto",
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
    toolchains = ["@rules_go//go:toolchain"],
)


def go_srcs_tar(name, deps, visibility = None):
  """Outputs a .tar file containing the source files of go_library targets."""
  output_go_library_srcs(
      name = name,
      deps = deps,
      out = name + ".tar",
      visibility = ["//visibility:private"],
  )
