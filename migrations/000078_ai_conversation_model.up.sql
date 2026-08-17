ALTER TABLE ai.conversations
    ADD COLUMN model_id text;

UPDATE ai.conversations AS conversation
SET model_id = latest_turn.model_id
FROM (
    SELECT DISTINCT ON (conversation_id)
        conversation_id,
        model_id
    FROM ai.turns
    WHERE model_id IS NOT NULL
    ORDER BY conversation_id, turn_index DESC
) AS latest_turn
WHERE conversation.id = latest_turn.conversation_id
  AND conversation.model_id IS NULL;
