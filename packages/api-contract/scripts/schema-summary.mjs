const DEFAULT_MAX_DEPTH = 6;

export function createSchemaSummarizer(document, options = {}) {
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;
  if (!Number.isInteger(maxDepth) || maxDepth < 1) {
    throw new TypeError("maxDepth must be a positive integer");
  }

  return function summarizeSchema(schema) {
    return summarize(schema, {
      depth: 0,
      referenceStack: new Set(),
    });
  };

  function summarize(schema, state) {
    if (typeof schema === "boolean") {
      return { allowed: schema };
    }
    if (!schema || typeof schema !== "object") {
      return undefined;
    }

    const ref = typeof schema.$ref === "string" ? schema.$ref : undefined;
    if (ref) {
      const referenceSummary = { ref };
      if (!ref.startsWith("#/")) {
        return referenceSummary;
      }
      if (state.referenceStack.has(ref)) {
        return { ...referenceSummary, circular: true };
      }
      if (state.depth >= maxDepth) {
        return { ...referenceSummary, truncated: true };
      }

      const resolved = resolveLocalReference(document, ref);
      if (!resolved) {
        return referenceSummary;
      }

      const referenceStack = new Set(state.referenceStack);
      referenceStack.add(ref);
      const resolvedSummary = summarize(resolved, {
        depth: state.depth + 1,
        referenceStack,
      });
      const siblingSummary = summarizeSchemaObject(
        Object.fromEntries(
          Object.entries(schema).filter(([key]) => key !== "$ref"),
        ),
        state,
      );
      return compact({
        ...referenceSummary,
        ...resolvedSummary,
        ...siblingSummary,
      });
    }

    return summarizeSchemaObject(schema, state);
  }

  function summarizeSchemaObject(schema, state) {
    const hasNestedSchema = Boolean(
      schema.properties
      || schema.items
      || schema.additionalProperties
      || schema.allOf
      || schema.anyOf
      || schema.oneOf
      || schema.not,
    );
    if (state.depth >= maxDepth && hasNestedSchema) {
      return compact({
        type: normalizedType(schema.type, schema.properties),
        format: schema.format,
        enum: schema.enum,
        nullable: schema.nullable,
        truncated: true,
      });
    }

    const nestedState = {
      ...state,
      depth: state.depth + 1,
    };
    const properties = schema.properties && typeof schema.properties === "object"
      ? Object.fromEntries(
        Object.entries(schema.properties)
          .sort(([left], [right]) => left.localeCompare(right))
          .map(([name, propertySchema]) => [
            name,
            summarize(propertySchema, nestedState) ?? {},
          ]),
      )
      : undefined;

    return compact({
      type: normalizedType(schema.type, properties),
      format: schema.format,
      title: schema.title,
      description: schema.description,
      enum: schema.enum,
      const: schema.const,
      default: schema.default,
      nullable: schema.nullable,
      readOnly: schema.readOnly,
      writeOnly: schema.writeOnly,
      required: Array.isArray(schema.required)
        ? [...schema.required].sort()
        : undefined,
      properties,
      items: summarize(schema.items, nestedState),
      additionalProperties: summarizeAdditionalProperties(
        schema.additionalProperties,
        nestedState,
      ),
      allOf: summarizeComposition(schema.allOf, nestedState),
      anyOf: summarizeComposition(schema.anyOf, nestedState),
      oneOf: summarizeComposition(schema.oneOf, nestedState),
      not: summarize(schema.not, nestedState),
      minLength: schema.minLength,
      maxLength: schema.maxLength,
      pattern: schema.pattern,
      minimum: schema.minimum,
      maximum: schema.maximum,
      minItems: schema.minItems,
      maxItems: schema.maxItems,
      uniqueItems: schema.uniqueItems,
    });
  }

  function summarizeAdditionalProperties(value, state) {
    if (typeof value === "boolean") {
      return value;
    }
    return summarize(value, state);
  }

  function summarizeComposition(value, state) {
    if (!Array.isArray(value)) {
      return undefined;
    }
    return value.map((item) => summarize(item, state) ?? {});
  }
}

export function resolveLocalReference(document, ref) {
  if (typeof ref !== "string" || !ref.startsWith("#/")) {
    return undefined;
  }
  return ref
    .slice(2)
    .split("/")
    .map(part => part.replaceAll("~1", "/").replaceAll("~0", "~"))
    .reduce((current, part) => current?.[part], document);
}

function normalizedType(type, properties) {
  if (typeof type === "string") {
    return type;
  }
  if (Array.isArray(type)) {
    return type;
  }
  return properties ? "object" : undefined;
}

function compact(value) {
  return Object.fromEntries(
    Object.entries(value).filter(([, entry]) => entry !== undefined),
  );
}
