/** Scope generated CSS, including Tailwind's reset, without changing host CSS. */
export function scopePluginCss() {
  return {
    postcssPlugin: "media-workflow-plugin-scope",
    OnceExit(root) {
      const animations = new Map();
      root.walkAtRules(/keyframes$/i, (rule) => {
        animations.set(rule.params, `media-workflow-${rule.params}`);
        rule.params = animations.get(rule.params);
      });
      root.walkDecls(
        /^(animation|animation-name|--tw-enter|--tw-exit)/,
        (decl) => {
          for (const [name, scoped] of animations) {
            decl.value = decl.value.replace(
              new RegExp(
                `(^|[\\s,])${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(?=$|[\\s,])`,
                "g",
              ),
              `$1${scoped}`,
            );
          }
        },
      );
      root.walkRules((rule) => {
        if (
          rule.parent?.type === "atrule" &&
          /keyframes$/i.test(rule.parent.name)
        )
          return;
        rule.selectors = rule.selectors.map((selector) => {
          if (selector.startsWith(".media-workflow-ui")) return selector;
          if (/^(:root|html|body)$/.test(selector)) return ".media-workflow-ui";
          return `:where(.media-workflow-ui) ${selector}`;
        });
      });
    },
  };
}
