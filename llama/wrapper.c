#include "wrapper.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <math.h>

static int g_log_level = 2;

static void jsonify_log_callback(enum ggml_log_level level, const char * text, void * user_data) {
    (void)user_data;
    if (level >= g_log_level) {
        fputs(text, stderr);
    }
}

void jsonify_set_log_level(int32_t level) {
    g_log_level = level;
    llama_log_set(jsonify_log_callback, NULL);
}

jsonify_model_t * jsonify_load(const char * model_path, int32_t n_ctx, int32_t n_threads, int32_t n_gpu_layers) {
    struct llama_model_params mparams = llama_model_default_params();
    mparams.n_gpu_layers = n_gpu_layers;
    mparams.use_mmap = true;

    struct llama_model * model = llama_model_load_from_file(model_path, mparams);
    if (!model) {
        fprintf(stderr, "Failed to load model from %s\n", model_path);
        return NULL;
    }

    struct llama_context_params cparams = llama_context_default_params();
    if (n_ctx > 0) cparams.n_ctx = n_ctx;
    if (n_threads > 0) {
        cparams.n_threads = n_threads;
        cparams.n_threads_batch = n_threads;
    }

    struct llama_context * ctx = llama_init_from_model(model, cparams);
    if (!ctx) {
        fprintf(stderr, "Failed to create context\n");
        llama_model_free(model);
        return NULL;
    }

    struct llama_sampler_chain_params sparams = llama_sampler_chain_default_params();
    struct llama_sampler * chain = llama_sampler_chain_init(sparams);
    llama_sampler_chain_add(chain, llama_sampler_init_greedy());

    const struct llama_vocab * vocab = llama_model_get_vocab(model);
    int32_t n_vocab = llama_vocab_n_tokens(vocab);

    jsonify_model_t * m = (jsonify_model_t *)calloc(1, sizeof(jsonify_model_t));
    m->model = model;
    m->ctx = ctx;
    m->chain = chain;
    m->grammar = NULL;
    m->vocab = vocab;
    m->n_vocab = n_vocab;
    m->n_ctx = (int32_t)llama_n_ctx(ctx);
    return m;
}

void jsonify_free(jsonify_model_t * m) {
    if (!m) return;
    if (m->grammar) llama_sampler_free(m->grammar);
    if (m->chain) llama_sampler_free(m->chain);
    if (m->ctx) llama_free(m->ctx);
    if (m->model) llama_model_free(m->model);
    free(m);
}

int32_t jsonify_tokenize(jsonify_model_t * m, const char * text, int32_t * tokens_buf, int32_t capacity, bool add_bos, bool parse_special) {
    return llama_tokenize(m->vocab, text, (int32_t)strlen(text), tokens_buf, capacity, add_bos, parse_special);
}

int32_t jsonify_decode(jsonify_model_t * m, const int32_t * tokens, int32_t n_tokens) {
    struct llama_batch batch = llama_batch_get_one((llama_token *)tokens, n_tokens);
    int32_t ret = llama_decode(m->ctx, batch);
    return ret;
}

void jsonify_synchronize(jsonify_model_t * m) {
    llama_synchronize(m->ctx);
}

int32_t jsonify_sample_next(jsonify_model_t * m) {
    int32_t n_vocab = m->n_vocab;

    // Get logits
    float * logits = llama_get_logits(m->ctx);

    // Build token data array
    llama_token_data * data = (llama_token_data *)malloc(n_vocab * sizeof(llama_token_data));
    for (int32_t i = 0; i < n_vocab; i++) {
        data[i].id = i;
        data[i].logit = logits[i];
        data[i].p = 0.0f;
    }
    llama_token_data_array cur_p = { data, (size_t)n_vocab, -1, false };

    // Apply grammar first (masks invalid tokens to -INFINITY)
    if (m->grammar) {
        llama_sampler_apply(m->grammar, &cur_p);
    }

    // Apply main chain (greedy picks argmax)
    llama_sampler_apply(m->chain, &cur_p);

    llama_token id = cur_p.data[cur_p.selected].id;
    free(data);
    return id;
}

void jsonify_accept_token(jsonify_model_t * m, int32_t token) {
    if (m->grammar) {
        llama_sampler_accept(m->grammar, token);
    }
    llama_sampler_accept(m->chain, token);
}

int32_t jsonify_n_vocab(jsonify_model_t * m) {
    return m->n_vocab;
}

int32_t jsonify_token_eos(jsonify_model_t * m) {
    return llama_vocab_eos(m->vocab);
}

int32_t jsonify_token_to_piece(jsonify_model_t * m, int32_t token, char * buf, int32_t buf_len) {
    return llama_token_to_piece(m->vocab, token, buf, buf_len, 0, true);
}

int32_t jsonify_set_grammar(jsonify_model_t * m, const char * grammar_str, const char * grammar_root) {
    if (m->grammar) {
        llama_sampler_free(m->grammar);
        m->grammar = NULL;
    }
    m->grammar = llama_sampler_init_grammar(m->vocab, grammar_str, grammar_root);
    if (!m->grammar) {
        return -1;
    }
    return 0;
}

void jsonify_clear_grammar(jsonify_model_t * m) {
    if (m->grammar) {
        llama_sampler_free(m->grammar);
        m->grammar = NULL;
    }
}

void jsonify_sampler_free(struct llama_sampler * smpl) {
    if (smpl) llama_sampler_free(smpl);
}

char *jsonify_chat_apply_template(jsonify_model_t * m,
                                  const char * system_msg,
                                  const char * user_msg,
                                  bool add_ass) {
    if (!m || !m->model) return NULL;

    struct llama_chat_message msgs[2];
    int n_msg = 0;

    if (system_msg && strlen(system_msg) > 0) {
        msgs[n_msg].role = "system";
        msgs[n_msg].content = system_msg;
        n_msg++;
    }
    if (user_msg && strlen(user_msg) > 0) {
        msgs[n_msg].role = "user";
        msgs[n_msg].content = user_msg;
        n_msg++;
    }
    if (n_msg == 0) return NULL;

    int32_t len = llama_chat_apply_template(NULL, msgs, n_msg, add_ass, NULL, 0);
    if (len <= 0) return NULL;

    char *buf = (char *)malloc(len + 1);
    if (!buf) return NULL;

    int32_t ret = llama_chat_apply_template(NULL, msgs, n_msg, add_ass, buf, len + 1);
    if (ret <= 0) {
        free(buf);
        return NULL;
    }
    return buf;
}
